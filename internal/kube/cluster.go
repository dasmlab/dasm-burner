package kube

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

// Snapshot is observed object counts for a managed run.
type Snapshot struct {
	Namespaces  int            `json:"namespaces"`
	Services    int            `json:"services"`
	Routes      int            `json:"routes"`
	Deployments int            `json:"deployments"`
	Pods        int            `json:"pods"`
	ReadyPods   int            `json:"readyPods"`
	PodPhases   map[string]int `json:"podPhases,omitempty"`
}

// Cluster is the apply/observe surface. Tests use Fake; apply uses Live.
type Cluster interface {
	CreateNamespace(ctx context.Context, ns *corev1.Namespace) error
	CreateService(ctx context.Context, svc *corev1.Service) error
	CreateDeployment(ctx context.Context, d *appsv1.Deployment) error
	CreateRoute(ctx context.Context, rt *topology.Route) error
	DeleteNamespace(ctx context.Context, name string) error

	CopySecret(ctx context.Context, srcNamespace, name, dstNamespace string) error

	GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error)
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
	RouteAdmitted(ctx context.Context, namespace, name string) (bool, error)

	ListManaged(ctx context.Context, runID string) (Snapshot, error)
	ListManagedNamespaces(ctx context.Context, runID string) ([]string, error)

	ClusterHealth(ctx context.Context, runID string) (Health, error)

	HasRouteAPI(ctx context.Context) (bool, error)
	ServerVersion(ctx context.Context) (string, error)
}
