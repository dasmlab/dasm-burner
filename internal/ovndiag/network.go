package ovndiag

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Required OVN-Kubernetes node annotations observed on OCP 4.21 OVN clusters
// (e.g. k8s.ovn.org/node-subnets, l3-gateway-config). Missing keys ⇒ config drift.
func requiredOVNAnnots() []string {
	return []string{
		"k8s.ovn.org/node-subnets",
		"k8s.ovn.org/l3-gateway-config",
		"k8s.ovn.org/node-primary-ifaddr",
		"k8s.ovn.org/host-cidrs",
		"k8s.ovn.org/node-chassis-id",
	}
}

func readNetworkLayer(n corev1.Node) NetworkLayer {
	l := NetworkLayer{AnnotationsOK: true}
	anns := n.Annotations
	if anns == nil {
		anns = map[string]string{}
	}
	var missing []string
	for _, k := range requiredOVNAnnots() {
		v := strings.TrimSpace(anns[k])
		if v == "" {
			missing = append(missing, k)
		}
	}
	l.MissingAnnots = missing
	l.AnnotationsOK = len(missing) == 0
	if s := anns["k8s.ovn.org/node-subnets"]; s != "" {
		l.NodeSubnets = truncate(s, 120)
	}
	if s := anns["k8s.ovn.org/zone-name"]; s != "" {
		l.Zone = s
	}
	return l
}

func evaluateNetwork(n OVNNodeHealth, now time.Time, batchID int) []Finding {
	var out []Finding
	if n.Network.AnnotationsOK {
		return out
	}
	miss := strings.Join(n.Network.MissingAnnots, ", ")
	out = append(out, Finding{
		ID:        fmt.Sprintf("%s-%s-%d", RuleNetworkConfigDrift, n.NodeName, now.Unix()),
		RuleID:    RuleNetworkConfigDrift,
		Severity:  SevWarning,
		Category:  CatNetwork,
		Node:      n.NodeName,
		Component: "node-annotations",
		FirstSeen: now,
		LastSeen:  now,
		Count:     len(n.Network.MissingAnnots),
		Summary:   "OVN node annotations incomplete / drifted",
		Evidence: []Evidence{{
			Label:   "missing",
			Current: miss,
		}},
		BatchID: batchID,
		Why:     "Expected k8s.ovn.org/* programming annotations are absent — incomplete OVN node setup or drift.",
	})
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
