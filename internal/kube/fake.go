package kube

import (
	"context"
	"fmt"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

// Fake is an in-memory Cluster for tests. Creates become immediately ready
// unless NeverReady is set.
type Fake struct {
	mu          sync.Mutex
	NS          map[string]*corev1.Namespace
	Svc         map[string]*corev1.Service
	Dep         map[string]*appsv1.Deployment
	Rt          map[string]*topology.Route
	NeverReady  bool
	HasRoutes   bool
	Version     string
	CreateCalls int
	Health      Health
}

func NewFake() *Fake {
	return &Fake{
		NS:        map[string]*corev1.Namespace{},
		Svc:       map[string]*corev1.Service{},
		Dep:       map[string]*appsv1.Deployment{},
		Rt:        map[string]*topology.Route{},
		HasRoutes: true,
		Version:   "fake-1",
		Health:    Health{NodesReady: 3},
	}
}

func (f *Fake) ClusterHealth(ctx context.Context, runID string) (Health, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.Health
	h.SampledAt = h.SampledAt
	return h, nil
}

func key(ns, name string) string { return ns + "/" + name }

func alreadyExists(kind, name string) error {
	return apierrors.NewAlreadyExists(schema.GroupResource{Resource: kind}, name)
}

func notFound(kind, name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, name)
}

func (f *Fake) CreateNamespace(ctx context.Context, ns *corev1.Namespace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCalls++
	if _, ok := f.NS[ns.Name]; ok {
		return alreadyExists("namespaces", ns.Name)
	}
	cp := ns.DeepCopy()
	if !f.NeverReady {
		cp.Status.Phase = corev1.NamespaceActive
	}
	f.NS[ns.Name] = cp
	return nil
}

func (f *Fake) CreateService(ctx context.Context, svc *corev1.Service) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCalls++
	k := key(svc.Namespace, svc.Name)
	if _, ok := f.Svc[k]; ok {
		return alreadyExists("services", svc.Name)
	}
	f.Svc[k] = svc.DeepCopy()
	return nil
}

func (f *Fake) CreateDeployment(ctx context.Context, d *appsv1.Deployment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCalls++
	k := key(d.Namespace, d.Name)
	if _, ok := f.Dep[k]; ok {
		return alreadyExists("deployments", d.Name)
	}
	cp := d.DeepCopy()
	if !f.NeverReady && cp.Spec.Replicas != nil {
		r := *cp.Spec.Replicas
		cp.Status.Replicas = r
		cp.Status.ReadyReplicas = r
		cp.Status.AvailableReplicas = r
	}
	f.Dep[k] = cp
	return nil
}

func (f *Fake) CreateRoute(ctx context.Context, rt *topology.Route) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCalls++
	k := key(rt.Metadata.Namespace, rt.Metadata.Name)
	if _, ok := f.Rt[k]; ok {
		return alreadyExists("routes", rt.Metadata.Name)
	}
	cp := *rt
	f.Rt[k] = &cp
	return nil
}

func (f *Fake) CopySecret(ctx context.Context, srcNamespace, name, dstNamespace string) error {
	return nil
}

func (f *Fake) DeleteNamespace(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.NS, name)
	for k, svc := range f.Svc {
		if svc.Namespace == name {
			delete(f.Svc, k)
		}
	}
	for k, d := range f.Dep {
		if d.Namespace == name {
			delete(f.Dep, k)
		}
	}
	for k, rt := range f.Rt {
		if rt.Metadata.Namespace == name {
			delete(f.Rt, k)
		}
	}
	return nil
}

func (f *Fake) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ns, ok := f.NS[name]
	if !ok {
		return nil, notFound("namespaces", name)
	}
	return ns.DeepCopy(), nil
}

func (f *Fake) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.Dep[key(namespace, name)]
	if !ok {
		return nil, notFound("deployments", name)
	}
	return d.DeepCopy(), nil
}

func (f *Fake) RouteAdmitted(ctx context.Context, namespace, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Rt[key(namespace, name)]; !ok {
		return false, notFound("routes", name)
	}
	return !f.NeverReady, nil
}

func (f *Fake) ListManagedNamespaces(ctx context.Context, runID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var names []string
	for _, ns := range f.NS {
		if matchRun(ns.Labels, runID) {
			names = append(names, ns.Name)
		}
	}
	return names, nil
}

func (f *Fake) ListManaged(ctx context.Context, runID string) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var snap Snapshot
	for _, ns := range f.NS {
		if matchRun(ns.Labels, runID) {
			snap.Namespaces++
		}
	}
	for _, svc := range f.Svc {
		if matchRun(svc.Labels, runID) {
			snap.Services++
		}
	}
	for _, d := range f.Dep {
		if matchRun(d.Labels, runID) {
			snap.Deployments++
			if d.Spec.Replicas != nil {
				r := int(*d.Spec.Replicas)
				snap.Pods += r
				if d.Status.AvailableReplicas == *d.Spec.Replicas {
					snap.ReadyPods += r
				}
			}
		}
	}
	for _, rt := range f.Rt {
		if matchRun(rt.Metadata.Labels, runID) {
			snap.Routes++
		}
	}
	return snap, nil
}

func (f *Fake) HasRouteAPI(ctx context.Context) (bool, error) {
	return f.HasRoutes, nil
}

func (f *Fake) ServerVersion(ctx context.Context) (string, error) {
	if f.Version == "" {
		return "fake", nil
	}
	return f.Version, nil
}

func matchRun(labels map[string]string, runID string) bool {
	if labels[topology.LabelManaged] != "true" {
		return false
	}
	if runID != "" && labels[topology.LabelRun] != runID {
		return false
	}
	return true
}

func (f *Fake) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprintf("fake ns=%d svc=%d dep=%d rt=%d", len(f.NS), len(f.Svc), len(f.Dep), len(f.Rt))
}
