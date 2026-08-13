package ovndiag

import (
	"strings"

	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Caps are capability flags discovered from the live cluster (no OCP minor hard-coding).
type Caps struct {
	OVNNamespace    string
	HasOVNNamespace bool
	HasOVNKubeNode  bool
	HasControlPlane bool
	CanListEvents   bool
	CanReadPodLogs  bool
	NodeAnnotKeys   []string
	Capabilities    map[string]bool
}

// Discover probes what diagnostics we can actually run.
func Discover(ctx context.Context, cs kubernetes.Interface) Caps {
	c := Caps{
		OVNNamespace: ovnNS,
		Capabilities: map[string]bool{},
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, ovnNS, metav1.GetOptions{}); err == nil {
		c.HasOVNNamespace = true
		c.Capabilities["ovn_namespace"] = true
		c.Capabilities["l1_nodes"] = true
		c.Capabilities["l2_ovnkube"] = true
	}
	if c.HasOVNNamespace {
		pods, err := cs.CoreV1().Pods(ovnNS).List(ctx, metav1.ListOptions{Limit: 50})
		if err == nil {
			for _, p := range pods.Items {
				if containsOVNKubeNode(p.Name) {
					c.HasOVNKubeNode = true
					c.Capabilities["ovnkube_node"] = true
				}
				if containsControlPlane(p.Name) {
					c.HasControlPlane = true
					c.Capabilities["ovnkube_control_plane"] = true
				}
			}
		}
		if _, err := cs.CoreV1().Events(ovnNS).List(ctx, metav1.ListOptions{Limit: 1}); err == nil {
			c.CanListEvents = true
			c.Capabilities["events"] = true
		}
		c.CanReadPodLogs = true
		c.Capabilities["pod_logs"] = true
		c.Capabilities["l5_network_annotations"] = true
		c.NodeAnnotKeys = requiredOVNAnnots()
	}
	return c
}

func containsOVNKubeNode(name string) bool {
	return strings.HasPrefix(name, "ovnkube-node") || strings.Contains(name, "ovnkube-node")
}

func containsControlPlane(name string) bool {
	return strings.Contains(name, "ovnkube-control-plane") || strings.Contains(name, "ovnkube-master")
}
