package kube

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

// ManagedReaper deletes labeled burn objects even when the Namespace is already gone
// (force-finalize orphans). Live implements this; Fake does not have to.
type ManagedReaper interface {
	ReapLabeled(ctx context.Context, runID string, dryRun bool, log func(string)) (int, error)
}

func (l *Live) ReapLabeled(ctx context.Context, runID string, dryRun bool, log func(string)) (int, error) {
	if l == nil || l.cs == nil {
		return 0, fmt.Errorf("live client required")
	}
	if log == nil {
		log = func(string) {}
	}
	return ReapLabeled(ctx, l.cs, l.dyn, runID, dryRun, 4, log)
}

type namedNS struct {
	Kind      string
	Namespace string
	Name      string
}

// ReapLabeled deletes deployments, pods, services, and routes with the managed label.
// Namespace objects may already be gone; the objects still live in etcd.
func ReapLabeled(ctx context.Context, cs kubernetes.Interface, dyn dynamic.Interface, runID string, dryRun bool, workers int, log func(string)) (int, error) {
	if workers < 1 {
		workers = 1
	}
	if workers > 8 {
		workers = 8
	}
	sel := topology.Selector(runID)
	var objs []namedNS

	if deps, err := cs.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel}); err != nil {
		return 0, fmt.Errorf("list deployments: %w", err)
	} else {
		for _, d := range deps.Items {
			objs = append(objs, namedNS{"deploy", d.Namespace, d.Name})
		}
	}
	if svcs, err := cs.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel}); err != nil {
		return 0, fmt.Errorf("list services: %w", err)
	} else {
		for _, s := range svcs.Items {
			objs = append(objs, namedNS{"svc", s.Namespace, s.Name})
		}
	}
	if pods, err := cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel}); err != nil {
		return 0, fmt.Errorf("list pods: %w", err)
	} else {
		for _, p := range pods.Items {
			objs = append(objs, namedNS{"pod", p.Namespace, p.Name})
		}
	}
	if dyn != nil {
		if rts, err := dyn.Resource(routeGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel}); err == nil {
			for _, r := range rts.Items {
				objs = append(objs, namedNS{"route", r.GetNamespace(), r.GetName()})
			}
		}
	}

	log(fmt.Sprintf("orphan reap: %d labeled object(s) (deploy/svc/pod/route) run=%s", len(objs), orAll(runID)))
	if len(objs) == 0 {
		return 0, nil
	}
	if dryRun {
		log("dry-run — skip orphan delete")
		return 0, nil
	}

	zero := int64(0)
	delOpts := metav1.DeleteOptions{GracePeriodSeconds: &zero}
	var n atomic.Int64
	ch := make(chan namedNS)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for o := range ch {
				if ctx.Err() != nil {
					return
				}
				err := deleteLabeled(ctx, cs, dyn, o, delOpts)
				if err != nil && !apierrors.IsNotFound(err) {
					log("FAILED orphan " + o.Kind + " " + o.Namespace + "/" + o.Name + ": " + err.Error())
					continue
				}
				n.Add(1)
			}
		}()
	}
	for _, o := range objs {
		select {
		case <-ctx.Done():
			close(ch)
			wg.Wait()
			return int(n.Load()), ctx.Err()
		case ch <- o:
		}
	}
	close(ch)
	wg.Wait()
	deleted := int(n.Load())
	// Stuck Terminating pods in a missing Namespace need finalizers stripped.
	if left, err := cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel}); err == nil {
		for i := range left.Items {
			p := left.Items[i]
			if len(p.Finalizers) == 0 {
				_ = cs.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, delOpts)
				continue
			}
			p.Finalizers = nil
			if _, err := cs.CoreV1().Pods(p.Namespace).Update(ctx, &p, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
				log("FAILED clear finalizers " + p.Namespace + "/" + p.Name + ": " + err.Error())
				continue
			}
			_ = cs.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, delOpts)
			deleted++
		}
	}
	log(fmt.Sprintf("orphan reap issued %d delete(s)", deleted))
	return deleted, nil
}

func deleteLabeled(ctx context.Context, cs kubernetes.Interface, dyn dynamic.Interface, o namedNS, opts metav1.DeleteOptions) error {
	switch o.Kind {
	case "deploy":
		return cs.AppsV1().Deployments(o.Namespace).Delete(ctx, o.Name, opts)
	case "svc":
		return cs.CoreV1().Services(o.Namespace).Delete(ctx, o.Name, metav1.DeleteOptions{})
	case "pod":
		return cs.CoreV1().Pods(o.Namespace).Delete(ctx, o.Name, opts)
	case "route":
		if dyn == nil {
			return nil
		}
		return dyn.Resource(routeGVR).Namespace(o.Namespace).Delete(ctx, o.Name, metav1.DeleteOptions{})
	default:
		return fmt.Errorf("unknown kind %s", o.Kind)
	}
}

func orAll(runID string) string {
	if runID == "" {
		return "all"
	}
	return runID
}
