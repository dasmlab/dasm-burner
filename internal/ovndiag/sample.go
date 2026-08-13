package ovndiag

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const ovnNS = "openshift-ovn-kubernetes"

// Sample builds a live Snapshot (L1 node + L2 OVN-Kube pods) against baseline.
func Sample(ctx context.Context, cs kubernetes.Interface, baseline *Baseline, runID, cluster string, batchID int) (*Snapshot, error) {
	snap := &Snapshot{
		GeneratedAt:  time.Now(),
		RunID:        runID,
		Cluster:      cluster,
		Capabilities: map[string]bool{"l1_nodes": true, "l2_ovnkube": true},
	}
	if baseline != nil && !baseline.At().IsZero() {
		snap.BaselineAt = baseline.At()
	}

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ovnPods, err := cs.CoreV1().Pods(ovnNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	byNode := map[string]corev1.Pod{}
	for _, p := range ovnPods.Items {
		if !strings.HasPrefix(p.Name, "ovnkube-node-") && !strings.Contains(p.Name, "ovnkube-node") {
			// Prefer ovnkube-node; also keep first OVN pod on node as fallback
			if _, ok := byNode[p.Spec.NodeName]; ok {
				continue
			}
		}
		if p.Spec.NodeName != "" {
			byNode[p.Spec.NodeName] = p
		}
	}

	var findings []Finding
	now := time.Now()
	for _, n := range nodes.Items {
		nh := OVNNodeHealth{NodeName: n.Name, OverallState: StateHealthy}
		nh.Node = readNodeLayer(n)
		if p, ok := byNode[n.Name]; ok {
			nh.OVNKube = readOVNKubeLayer(p, baseline)
		}
		nf := evaluateNode(nh, now, batchID)
		nh.Findings = nf
		nh.OverallState = worstState(StateHealthy, severitiesToState(nf)...)
		findings = append(findings, nf...)
		snap.Nodes = append(snap.Nodes, nh)
		switch nh.OverallState {
		case StateHealthy:
			snap.HealthyCount++
		case StateWarning, StateDegraded:
			snap.WarningCount++
		case StateCritical, StateFailed:
			snap.CriticalCount++
		}
	}
	snap.Findings = findings
	snap.OverallState = aggregateState(snap.Nodes)
	snap.Timeline = buildTimeline(findings, batchID, now)
	snap.Why = why(snap)
	return snap, nil
}

func readNodeLayer(n corev1.Node) NodeLayer {
	l := NodeLayer{}
	for _, c := range n.Status.Conditions {
		switch c.Type {
		case corev1.NodeReady:
			l.Ready = c.Status == corev1.ConditionTrue
			l.LastReadyChange = c.LastTransitionTime.Time
		case corev1.NodeMemoryPressure:
			l.MemoryPressure = c.Status == corev1.ConditionTrue
		case corev1.NodeDiskPressure:
			l.DiskPressure = c.Status == corev1.ConditionTrue
		case corev1.NodePIDPressure:
			l.PIDPressure = c.Status == corev1.ConditionTrue
		case corev1.NodeNetworkUnavailable:
			l.NetworkUnavailable = c.Status == corev1.ConditionTrue
		}
	}
	return l
}

func readOVNKubeLayer(p corev1.Pod, baseline *Baseline) OVNKubeLayer {
	l := OVNKubeLayer{
		PodName: p.Name,
		Phase:   string(p.Status.Phase),
		Ready:   podReady(p),
	}
	for _, cs := range p.Status.ContainerStatuses {
		l.Restarts += int(cs.RestartCount)
		if cs.State.Waiting != nil && strings.Contains(cs.State.Waiting.Reason, "CrashLoop") {
			l.CrashLoop = true
		}
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			l.OOMKilled = true
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
			l.OOMKilled = true
		}
	}
	if baseline != nil {
		if prev, ok := baseline.RestartWatermark(p.Name); ok {
			d := l.Restarts - prev
			if d < 0 {
				d = 0
			}
			l.RestartsDelta = d
		}
	}
	return l
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func evaluateNode(n OVNNodeHealth, now time.Time, batchID int) []Finding {
	var out []Finding
	add := func(rule string, sev Severity, cat Category, summary string, ev ...Evidence) {
		out = append(out, Finding{
			ID:        fmt.Sprintf("%s-%s-%d", rule, n.NodeName, now.Unix()),
			RuleID:    rule,
			Severity:  sev,
			Category:  cat,
			Node:      n.NodeName,
			Component: "ovnkube-node",
			FirstSeen: now,
			LastSeen:  now,
			Count:     1,
			Summary:   summary,
			Evidence:  ev,
			BatchID:   batchID,
		})
	}
	if !n.Node.Ready {
		add(RuleNodeNotReady, SevCritical, CatNode, "Node Ready=False",
			Evidence{Label: "Ready", Current: "false"})
	}
	if n.Node.MemoryPressure {
		add(RuleMemoryPressure, SevWarning, CatNode, "Node MemoryPressure=True")
	}
	if n.Node.DiskPressure {
		add(RuleDiskPressure, SevWarning, CatNode, "Node DiskPressure=True")
	}
	if n.Node.PIDPressure {
		add(RulePIDPressure, SevWarning, CatNode, "Node PIDPressure=True")
	}
	if n.Node.NetworkUnavailable {
		add(RuleNetworkUnavailable, SevError, CatNode, "Node NetworkUnavailable=True")
	}
	if n.OVNKube.PodName == "" {
		add(RuleOVNKubeNotReady, SevWarning, CatOVNKube, "No ovnkube-node pod mapped to this node")
	} else {
		if !n.OVNKube.Ready {
			add(RuleOVNKubeNotReady, SevError, CatOVNKube, "ovnkube-node not Ready",
				Evidence{Label: "pod", Current: n.OVNKube.PodName})
		}
		if n.OVNKube.RestartsDelta > 0 {
			add(RuleOVNKubeRestart, SevWarning, CatOVNKube,
				fmt.Sprintf("ovnkube-node restart Δ=%d during window", n.OVNKube.RestartsDelta),
				Evidence{Label: "restarts", Current: fmt.Sprintf("%d", n.OVNKube.Restarts), Delta: fmt.Sprintf("+%d", n.OVNKube.RestartsDelta)})
		}
		if n.OVNKube.CrashLoop {
			add(RuleOVNKubeCrashLoop, SevCritical, CatOVNKube, "ovnkube-node CrashLoopBackOff")
		}
		if n.OVNKube.OOMKilled {
			add(RuleOVNKubeOOM, SevCritical, CatOVNKube, "ovnkube-node OOMKilled")
		}
	}
	return out
}

func severitiesToState(fs []Finding) []HealthState {
	var out []HealthState
	for _, f := range fs {
		switch f.Severity {
		case SevCritical, SevError:
			out = append(out, StateCritical)
		case SevWarning:
			out = append(out, StateWarning)
		case SevNotice:
			out = append(out, StateDegraded)
		}
	}
	return out
}

func worstState(base HealthState, others ...HealthState) HealthState {
	rank := map[HealthState]int{
		StateHealthy: 0, StateDegraded: 1, StateWarning: 2, StateCritical: 3, StateFailed: 4,
	}
	w := base
	for _, o := range others {
		if rank[o] > rank[w] {
			w = o
		}
	}
	return w
}

func aggregateState(nodes []OVNNodeHealth) HealthState {
	w := StateHealthy
	for _, n := range nodes {
		w = worstState(w, n.OverallState)
	}
	return w
}

func buildTimeline(fs []Finding, batchID int, now time.Time) []TimelineEvent {
	var tl []TimelineEvent
	if batchID > 0 {
		tl = append(tl, TimelineEvent{At: now, Kind: "batch", Summary: fmt.Sprintf("batch %d marker", batchID), BatchID: batchID})
	}
	for _, f := range fs {
		if f.Severity == SevInfo {
			continue
		}
		tl = append(tl, TimelineEvent{
			At: f.LastSeen, Kind: "finding", Summary: f.Summary, Node: f.Node,
			BatchID: f.BatchID, Finding: f.ID, Severity: f.Severity,
		})
	}
	return tl
}

func why(s *Snapshot) string {
	if s.OverallState == StateHealthy {
		return "All observed nodes and ovnkube-node pods look healthy vs baseline."
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("Overall %s — %d healthy, %d warning, %d critical.",
		s.OverallState, s.HealthyCount, s.WarningCount, s.CriticalCount))
	n := 0
	for _, f := range s.Findings {
		if f.Severity == SevWarning || f.Severity == SevError || f.Severity == SevCritical {
			parts = append(parts, fmt.Sprintf("%s on %s: %s", f.RuleID, f.Node, f.Summary))
			n++
			if n >= 5 {
				break
			}
		}
	}
	return strings.Join(parts, " ")
}
