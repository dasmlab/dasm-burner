package ovndiag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DataplaneLayer is L6: gateway programming, OVS daemon Ready, CNI sandbox failures.
type DataplaneLayer struct {
	OVSReady        bool   `json:"ovsReady"`
	OVSPresent      bool   `json:"ovsPresent"`
	GatewayOK       bool   `json:"gatewayOK"`
	GatewayMode     string `json:"gatewayMode,omitempty"`
	SandboxFailures int    `json:"sandboxFailures"`
	PendingNoIP     int    `json:"pendingNoIP"`
	Present         bool   `json:"present"`
}

type dataplaneSignals struct {
	sandbox map[string]nodeSignal
	pending map[string]int
}

type nodeSignal struct {
	Count  int
	Sample string
}

var ovsContainerNames = []string{"ovs-daemons", "ovs-vswitchd", "openvswitch", "ovsdb-server"}

func collectDataplaneSignals(ctx context.Context, cs kubernetes.Interface, window time.Duration) dataplaneSignals {
	sig := dataplaneSignals{
		sandbox: map[string]nodeSignal{},
		pending: map[string]int{},
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	cutoff := time.Now().Add(-window)

	evs, err := cs.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "reason=FailedCreatePodSandBox",
		Limit:         300,
	})
	if err == nil {
		for _, e := range evs.Items {
			if eventTime(e).Before(cutoff) {
				continue
			}
			node := dataplaneEventNode(e)
			if node == "" {
				continue
			}
			n := e.Count
			if n < 1 {
				n = 1
			}
			cur := sig.sandbox[node]
			cur.Count += int(n)
			if cur.Sample == "" {
				cur.Sample = truncate(e.Message, 160)
			}
			sig.sandbox[node] = cur
		}
	}

	pending, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase=Pending",
		Limit:         400,
	})
	if err == nil {
		for _, p := range pending.Items {
			if p.Spec.HostNetwork || p.Spec.NodeName == "" {
				continue
			}
			if podHasIP(p) {
				continue
			}
			if !pendingLooksNetwork(p) {
				continue
			}
			sig.pending[p.Spec.NodeName]++
		}
	}
	return sig
}

func readDataplaneLayer(n corev1.Node, ovnPods []corev1.Pod, sig dataplaneSignals) DataplaneLayer {
	l := DataplaneLayer{Present: true, GatewayOK: true, OVSReady: true}
	l.GatewayOK, l.GatewayMode = parseGatewayConfig(n.Annotations["k8s.ovn.org/l3-gateway-config"])
	if s, ok := sig.sandbox[n.Name]; ok {
		l.SandboxFailures = s.Count
	}
	l.PendingNoIP = sig.pending[n.Name]
	ready, present := ovsDaemonStatus(ovnPods)
	l.OVSPresent = present
	l.OVSReady = !present || ready
	return l
}

func evaluateDataplane(n OVNNodeHealth, sig dataplaneSignals, now time.Time, batchID int) []Finding {
	var out []Finding
	d := n.Dataplane
	if !d.Present {
		return out
	}
	add := func(rule string, sev Severity, summary, why string, ev ...Evidence) {
		out = append(out, Finding{
			ID:     fmt.Sprintf("%s-%s-%d", rule, n.NodeName, now.Unix()),
			RuleID: rule, Severity: sev, Category: CatDataplane,
			Node: n.NodeName, Component: "dataplane", FirstSeen: now, LastSeen: now, Count: 1,
			Summary: summary, Evidence: ev, BatchID: batchID, Why: why,
		})
	}
	if d.OVSPresent && !d.OVSReady {
		add(RuleOVSNotReady, SevError, "OVS daemon container not Ready",
			"ovs-vswitchd / ovs-daemons not Ready — br-int programming and Geneve dataplane stall on this node.")
	}
	if !d.GatewayOK {
		add(RuleGatewayInvalid, SevWarning, "OVN L3 gateway config incomplete",
			"k8s.ovn.org/l3-gateway-config missing mode, next-hop, or IP — east-west may work while egress/ingress on this node is broken.",
			Evidence{Label: "mode", Current: d.GatewayMode})
	}
	if d.SandboxFailures > 0 {
		sev := SevWarning
		if d.SandboxFailures >= 5 {
			sev = SevError
		}
		sample := sig.sandbox[n.NodeName].Sample
		add(RuleSandboxFail, sev,
			fmt.Sprintf("FailedCreatePodSandBox ×%d in window", d.SandboxFailures),
			"Kubelet could not set up the pod network sandbox — CNI/OVN dataplane connectivity failure on this node.",
			Evidence{Label: "count", Current: fmt.Sprintf("%d", d.SandboxFailures)},
			Evidence{Label: "sample", Current: sample})
	}
	if d.PendingNoIP > 0 {
		sev := SevWarning
		if d.PendingNoIP >= 8 {
			sev = SevError
		}
		add(RuleNoPodIP, sev,
			fmt.Sprintf("%d Pending pods with no PodIP (network-like)", d.PendingNoIP),
			"Pods scheduled here have no CNI address — dataplane did not complete setup.",
			Evidence{Label: "pendingNoIP", Current: fmt.Sprintf("%d", d.PendingNoIP)})
	}
	return out
}

func parseGatewayConfig(raw string) (ok bool, mode string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return false, ""
	}
	rawDef, has := top["default"]
	if !has {
		for _, v := range top {
			rawDef = v
			break
		}
	}
	if len(rawDef) == 0 {
		return false, ""
	}
	var gw struct {
		Mode        string   `json:"mode"`
		IPAddress   string   `json:"ip-address"`
		IPAddresses []string `json:"ip-addresses"`
		NextHop     string   `json:"next-hop"`
		NextHops    []string `json:"next-hops"`
	}
	if err := json.Unmarshal(rawDef, &gw); err != nil {
		return false, ""
	}
	mode = strings.TrimSpace(gw.Mode)
	hasIP := strings.TrimSpace(gw.IPAddress) != "" || len(gw.IPAddresses) > 0
	hasHop := strings.TrimSpace(gw.NextHop) != "" || len(gw.NextHops) > 0
	return mode != "" && hasIP && hasHop, mode
}

func ovsDaemonStatus(pods []corev1.Pod) (ready, present bool) {
	ready = true
	for _, p := range pods {
		for _, cs := range p.Status.ContainerStatuses {
			if !isOVSContainer(cs.Name) {
				continue
			}
			present = true
			if !cs.Ready {
				ready = false
			}
		}
	}
	if !present {
		return false, false
	}
	return ready, true
}

func isOVSContainer(name string) bool {
	low := strings.ToLower(name)
	for _, n := range ovsContainerNames {
		if low == n || strings.Contains(low, n) {
			return true
		}
	}
	return strings.Contains(low, "ovs")
}

func dataplaneEventNode(e corev1.Event) string {
	if h := strings.TrimSpace(e.Source.Host); h != "" {
		return h
	}
	if h := strings.TrimSpace(e.ReportingInstance); h != "" {
		// kubelet often reports as "<node>" or "kubelet-<node>"
		return strings.TrimPrefix(h, "kubelet-")
	}
	return ""
}

func podHasIP(p corev1.Pod) bool {
	if strings.TrimSpace(p.Status.PodIP) != "" {
		return true
	}
	for _, ip := range p.Status.PodIPs {
		if strings.TrimSpace(ip.IP) != "" {
			return true
		}
	}
	return false
}

func pendingLooksNetwork(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		blob := strings.ToLower(c.Reason + " " + c.Message)
		if strings.Contains(blob, "network") || strings.Contains(blob, "cni") ||
			strings.Contains(blob, "sandbox") || strings.Contains(blob, "ovn") {
			return true
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			blob := strings.ToLower(cs.State.Waiting.Reason + " " + cs.State.Waiting.Message)
			if strings.Contains(blob, "network") || strings.Contains(blob, "cni") ||
				strings.Contains(blob, "sandbox") {
				return true
			}
		}
	}
	// Scheduled, still Pending, no IP — treat as possible CNI stall.
	scheduled := false
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionTrue {
			scheduled = true
		}
	}
	return scheduled && !podHasIP(p)
}

func ovnPodsByNode(pods []corev1.Pod) map[string][]corev1.Pod {
	out := map[string][]corev1.Pod{}
	for _, p := range pods {
		if p.Spec.NodeName == "" {
			continue
		}
		out[p.Spec.NodeName] = append(out[p.Spec.NodeName], p)
	}
	return out
}
