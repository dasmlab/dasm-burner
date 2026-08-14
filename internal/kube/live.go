package kube

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

var routeGVR = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}

// Live talks to a real cluster via client-go + dynamic (OpenShift Routes).
type Live struct {
	cs  kubernetes.Interface
	dyn dynamic.Interface
}

// Clientset exposes the typed client for observe-only helpers (cleanup watch).
func (l *Live) Clientset() kubernetes.Interface {
	if l == nil {
		return nil
	}
	return l.cs
}

// Dynamic exposes the unstructured client (OpenShift MachineConfig / KubeletConfig).
func (l *Live) Dynamic() dynamic.Interface {
	if l == nil {
		return nil
	}
	return l.dyn
}

func (l *Live) CreateNamespace(ctx context.Context, ns *corev1.Namespace) error {
	_, err := l.cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func (l *Live) CreateService(ctx context.Context, svc *corev1.Service) error {
	_, err := l.cs.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	return err
}

func (l *Live) CreateDeployment(ctx context.Context, d *appsv1.Deployment) error {
	_, err := l.cs.AppsV1().Deployments(d.Namespace).Create(ctx, d, metav1.CreateOptions{})
	return err
}

func (l *Live) CreateRoute(ctx context.Context, rt *topology.Route) error {
	u, err := routeToUnstructured(rt)
	if err != nil {
		return err
	}
	_, err = l.dyn.Resource(routeGVR).Namespace(rt.Metadata.Namespace).Create(ctx, u, metav1.CreateOptions{})
	return err
}

func (l *Live) CopySecret(ctx context.Context, srcNamespace, name, dstNamespace string) error {
	src, err := l.cs.CoreV1().Secrets(srcNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("read secret %s/%s: %w", srcNamespace, name, err)
	}
	dst := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: dstNamespace,
			Labels: map[string]string{
				topology.LabelManaged: "true",
			},
		},
		Type: src.Type,
		Data: src.Data,
	}
	_, err = l.cs.CoreV1().Secrets(dstNamespace).Create(ctx, dst, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (l *Live) DeleteNamespace(ctx context.Context, name string) error {
	err := l.cs.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (l *Live) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	return l.cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
}

func (l *Live) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return l.cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (l *Live) RouteAdmitted(ctx context.Context, namespace, name string) (bool, error) {
	u, err := l.dyn.Resource(routeGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return routeAdmitted(u), nil
}

func (l *Live) ListManagedNamespaces(ctx context.Context, runID string) ([]string, error) {
	list, err := l.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: topology.Selector(runID)})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

func (l *Live) ListManaged(ctx context.Context, runID string) (Snapshot, error) {
	sel := topology.Selector(runID)
	var snap Snapshot

	nsList, err := l.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return snap, fmt.Errorf("list namespaces: %w", err)
	}
	snap.Namespaces = len(nsList.Items)

	svcList, err := l.cs.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return snap, fmt.Errorf("list services: %w", err)
	}
	snap.Services = len(svcList.Items)

	depList, err := l.cs.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return snap, fmt.Errorf("list deployments: %w", err)
	}
	snap.Deployments = len(depList.Items)

	podList, err := l.cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return snap, fmt.Errorf("list pods: %w", err)
	}
	snap.Pods = len(podList.Items)
	for _, p := range podList.Items {
		if podReady(p) {
			snap.ReadyPods++
		}
	}

	if ok, err := l.HasRouteAPI(ctx); err == nil && ok {
		rtList, err := l.dyn.Resource(routeGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel})
		if err != nil {
			return snap, fmt.Errorf("list routes: %w", err)
		}
		snap.Routes = len(rtList.Items)
	}
	return snap, nil
}

func (l *Live) HasRouteAPI(ctx context.Context) (bool, error) {
	_, err := l.dyn.Resource(routeGVR).Namespace("default").List(ctx, metav1.ListOptions{Limit: 1})
	if err == nil || apierrors.IsForbidden(err) {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func (l *Live) ServerVersion(ctx context.Context) (string, error) {
	info, err := l.cs.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

func routeToUnstructured(rt *topology.Route) (*unstructured.Unstructured, error) {
	b, err := json.Marshal(rt)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: m}, nil
}

func routeAdmitted(u *unstructured.Unstructured) bool {
	ingress, found, err := unstructured.NestedSlice(u.Object, "status", "ingress")
	if err != nil || !found {
		return false
	}
	for _, item := range ingress {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		conds, _, _ := unstructured.NestedSlice(m, "conditions")
		for _, c := range conds {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if fmt.Sprint(cm["type"]) == "Admitted" && fmt.Sprint(cm["status"]) == "True" {
				return true
			}
		}
	}
	return false
}

func podReady(p corev1.Pod) bool {
	if p.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
