package topology

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/naming"
)

func int32ptr(v int32) *int32 { return &v }

func BuildNamespace(g *Graph, ns Namespace) *corev1.Namespace {
	labels := CommonLabels(g.RunID, naming.KindNamespace)
	labels[LabelConfig] = g.ConfigName
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns.Name,
			Labels: labels,
		},
	}
}

func BuildService(g *Graph, ns Namespace, pair Pair, cfg *config.Config) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pair.Service,
			Namespace: ns.Name,
			Labels:    PairLabels(g.RunID, pair.App, naming.KindService),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{LabelApp: pair.App},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       cfg.Application.Port,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func pullPolicy(s string) corev1.PullPolicy {
	switch corev1.PullPolicy(s) {
	case corev1.PullAlways, corev1.PullNever, corev1.PullIfNotPresent:
		return corev1.PullPolicy(s)
	default:
		return corev1.PullIfNotPresent
	}
}

func BuildDeployment(g *Graph, ns Namespace, pair Pair, cfg *config.Config) *appsv1.Deployment {
	labels := PairLabels(g.RunID, pair.App, naming.KindDeployment)
	podLabels := PairLabels(g.RunID, pair.App, "pod")
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pair.Deployment,
			Namespace: ns.Name,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32ptr(int32(pair.Replicas)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{LabelApp: pair.App},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					ImagePullSecrets: imagePullSecrets(cfg),
					Containers: []corev1.Container{{
						Name:            "web",
						Image:           cfg.Application.Image,
						ImagePullPolicy: pullPolicy(cfg.Application.ImagePullPolicy),
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: cfg.Application.Port,
							Protocol:      corev1.ProtocolTCP,
						}},
						Env: []corev1.EnvVar{{
							Name: "POD_NAME",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
							},
						}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readyz",
									Port: intstr.FromString("http"),
								},
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromString("http"),
								},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1m"),
								corev1.ResourceMemory: resource.MustParse("8Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("32Mi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: boolptr(false),
							RunAsNonRoot:             boolptr(true),
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
					}},
				},
			},
		},
	}
}

func imagePullSecrets(cfg *config.Config) []corev1.LocalObjectReference {
	if cfg.Application.ImagePullSecret == "" {
		return nil
	}
	return []corev1.LocalObjectReference{{Name: cfg.Application.ImagePullSecret}}
}

func boolptr(v bool) *bool { return &v }

// Route is a minimal OpenShift Route. We keep this local so Phase 1 does not
// pull github.com/openshift/api just to emit YAML.
type Route struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"`
	Metadata   RouteMeta `json:"metadata"`
	Spec       RouteSpec `json:"spec"`
}

type RouteMeta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type RouteSpec struct {
	To             RouteTarget `json:"to"`
	Port           RoutePort   `json:"port"`
	TLS            *RouteTLS   `json:"tls,omitempty"`
	WildcardPolicy string      `json:"wildcardPolicy,omitempty"`
}

type RouteTarget struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Weight int32  `json:"weight"`
}

type RoutePort struct {
	TargetPort string `json:"targetPort"`
}

type RouteTLS struct {
	Termination                   string `json:"termination"`
	InsecureEdgeTerminationPolicy string `json:"insecureEdgeTerminationPolicy,omitempty"`
}

func BuildRoute(g *Graph, ns Namespace, pair Pair, cfg *config.Config) *Route {
	rt := &Route{
		APIVersion: "route.openshift.io/v1",
		Kind:       "Route",
		Metadata: RouteMeta{
			Name:      pair.Route,
			Namespace: ns.Name,
			Labels:    PairLabels(g.RunID, pair.App, naming.KindRoute),
		},
		Spec: RouteSpec{
			To: RouteTarget{
				Kind:   "Service",
				Name:   pair.Service,
				Weight: 100,
			},
			Port:           RoutePort{TargetPort: "http"},
			WildcardPolicy: "None",
		},
	}
	if cfg.Application.TLS.Enabled {
		rt.Spec.TLS = &RouteTLS{
			Termination:                   cfg.Application.TLS.Termination,
			InsecureEdgeTerminationPolicy: cfg.Application.TLS.InsecureEdgeTerminationPolicy,
		}
	}
	return rt
}
