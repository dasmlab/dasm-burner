package ovndiag

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const ovnNS = "openshift-ovn-kubernetes"

// SampleOpts controls expensive collectors (logs) and optional metrics.
type SampleOpts struct {
	ScanLogs    bool
	MaxLogPods  int
	EventWindow time.Duration
	Dyn         dynamic.Interface // metrics.k8s.io when available
}

// Sample builds a live Snapshot (L1/L2/L5 + events/logs + correlation) against baseline.
func Sample(ctx context.Context, cs kubernetes.Interface, baseline *Baseline, runID, cluster string, batchID int) (*Snapshot, error) {
	return SampleWith(ctx, cs, baseline, runID, cluster, batchID, SampleOpts{
		ScanLogs:    true,
		MaxLogPods:  6,
		EventWindow: 15 * time.Minute,
	})
}

// SampleLive is SampleWith plus optional dynamic client for metrics.k8s.io.
func SampleLive(ctx context.Context, cs kubernetes.Interface, dyn dynamic.Interface, baseline *Baseline, runID, cluster string, batchID int) (*Snapshot, error) {
	return SampleWith(ctx, cs, baseline, runID, cluster, batchID, SampleOpts{
		ScanLogs:    true,
		MaxLogPods:  6,
		EventWindow: 15 * time.Minute,
		Dyn:         dyn,
	})
}

func SampleWith(ctx context.Context, cs kubernetes.Interface, baseline *Baseline, runID, cluster string, batchID int, opts SampleOpts) (*Snapshot, error) {
	caps := Discover(ctx, cs)
	snap := &Snapshot{
		GeneratedAt:  time.Now(),
		RunID:        runID,
		Cluster:      cluster,
		Capabilities: caps.Capabilities,
	}
	if baseline != nil && !baseline.At().IsZero() {
		snap.BaselineAt = baseline.At()
	}
	if !caps.HasOVNNamespace {
		snap.OverallState = StateWarning
		snap.Why = "openshift-ovn-kubernetes namespace not found — OVN diagnoser cannot run L2+."
		snap.Findings = []Finding{{
			ID: fmt.Sprintf("OVN000-missing-ns-%d", time.Now().Unix()), RuleID: "OVN000",
			Severity: SevWarning, Category: CatOVNKube, Summary: "OVN namespace missing",
			FirstSeen: time.Now(), LastSeen: time.Now(), Count: 1,
		}}
		return snap, nil
	}

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ovnPods, err := cs.CoreV1().Pods(ovnNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	byNode := mapOVNPodsByNode(ovnPods.Items)

	metricsOK := false
	if opts.Dyn != nil {
		metricsOK = MetricsAPIAvailable(ctx, opts.Dyn)
		if metricsOK {
			caps.Capabilities["l3_pod_metrics"] = true
		}
	}
	caps.Capabilities["l4_ovn_db_containers"] = true
	caps.Capabilities["l6_dataplane"] = true

	window := opts.EventWindow
	if window <= 0 {
		window = 15 * time.Minute
	}
	dpSig := collectDataplaneSignals(ctx, cs, window)
	onNode := ovnPodsByNode(ovnPods.Items)

	var findings []Finding
	now := time.Now()
	hot := map[string]bool{}
	for _, n := range nodes.Items {
		nh := OVNNodeHealth{NodeName: n.Name, OverallState: StateHealthy}
		nh.Node = readNodeLayer(n)
		nh.Network = readNetworkLayer(n)
		if p, ok := byNode[n.Name]; ok {
			nh.OVNKube = readOVNKubeLayer(p, baseline)
			nh.Database = readDatabaseLayer(p)
			if metricsOK && opts.Dyn != nil {
				if samples, err := CollectPodResources(ctx, opts.Dyn, p.Name); err == nil {
					nh.OVNKube.Resources = samples
				}
			}
		}
		nh.Dataplane = readDataplaneLayer(n, onNode[n.Name], dpSig)
		nf := evaluateNode(nh, now, batchID, baseline)
		nf = append(nf, evaluateNetwork(nh, now, batchID)...)
		nf = append(nf, evaluateDatabase(nh, now, batchID)...)
		nf = append(nf, evaluateDataplane(nh, dpSig, now, batchID)...)
		nf = append(nf, evaluateResources(n.Name, nh.OVNKube.Resources, baseline, now, batchID)...)
		nh.Findings = nf
		nh.OverallState = worstState(StateHealthy, severitiesToState(nf)...)
		if nh.OverallState != StateHealthy {
			hot[n.Name] = true
		}
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

	findings = append(findings, controlPlaneFindings(ovnPods.Items, now, batchID)...)

	if caps.CanListEvents {
		evFindings, evTL := CollectOVNEvents(ctx, cs, window)
		findings = append(findings, evFindings...)
		snap.Timeline = append(snap.Timeline, evTL...)
	}
	if opts.ScanLogs && caps.CanReadPodLogs {
		logFindings := ScanOVNLogs(ctx, cs, ovnPods.Items, hot, opts.MaxLogPods)
		findings = append(findings, logFindings...)
	}

	corr := CorrelateBatch(findings, batchID, now)
	findings = append(findings, corr...)

	snap.Findings = findings
	snap.OverallState = aggregateState(snap.Nodes)
	// Cluster-level findings can raise overall severity
	snap.OverallState = worstState(snap.OverallState, severitiesToState(findings)...)
	snap.Timeline = append(snap.Timeline, buildTimeline(findings, batchID, now)...)
	snap.Why = why(snap)
	return snap, nil
}

func mapOVNPodsByNode(pods []corev1.Pod) map[string]corev1.Pod {
	byNode := map[string]corev1.Pod{}
	for _, p := range pods {
		if p.Spec.NodeName == "" {
			continue
		}
		cur, ok := byNode[p.Spec.NodeName]
		prefer := containsOVNKubeNode(p.Name)
		havePrefer := ok && containsOVNKubeNode(cur.Name)
		if !ok || (prefer && !havePrefer) {
			byNode[p.Spec.NodeName] = p
		}
	}
	return byNode
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

func evaluateNode(n OVNNodeHealth, now time.Time, batchID int, baseline *Baseline) []Finding {
	var out []Finding
	add := func(rule string, sev Severity, cat Category, summary string, whyMsg string, ev ...Evidence) {
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
			Why:       whyMsg,
		})
	}
	if !n.Node.Ready {
		add(RuleNodeNotReady, SevCritical, CatNode, "Node Ready=False",
			"Node condition Ready is False — OVN dataplane on this node is unreliable.",
			Evidence{Label: "Ready", Current: "false"})
	} else if baseline != nil {
		if was, ok := baseline.ReadyWatermark(n.NodeName); ok && !was {
			add(RuleReadyFlap, SevWarning, CatNode, "Node recovered to Ready after baseline NotReady",
				"Ready flapped relative to baseline watermark — retained as historical incident signal.")
		}
	}
	if n.Node.MemoryPressure {
		add(RuleMemoryPressure, SevWarning, CatNode, "Node MemoryPressure=True",
			"MemoryPressure can starve ovnkube-node / OVS.")
	}
	if n.Node.DiskPressure {
		add(RuleDiskPressure, SevWarning, CatNode, "Node DiskPressure=True",
			"DiskPressure risks OVN DB / log write failures.")
	}
	if n.Node.PIDPressure {
		add(RulePIDPressure, SevWarning, CatNode, "Node PIDPressure=True",
			"PIDPressure can prevent OVN/OVS process recovery.")
	}
	if n.Node.NetworkUnavailable {
		add(RuleNetworkUnavailable, SevError, CatNode, "Node NetworkUnavailable=True",
			"CNI reports network unavailable on this node.")
	}
	if n.OVNKube.PodName == "" {
		add(RuleOVNKubeNotReady, SevWarning, CatOVNKube, "No ovnkube-node pod mapped to this node",
			"Dynamic discovery found no ovnkube-node on this node.")
	} else {
		if !n.OVNKube.Ready {
			add(RuleOVNKubeNotReady, SevError, CatOVNKube, "ovnkube-node not Ready",
				"ovnkube-node Ready condition is False.",
				Evidence{Label: "pod", Current: n.OVNKube.PodName})
		}
		if n.OVNKube.RestartsDelta > 0 {
			add(RuleOVNKubeRestart, SevWarning, CatOVNKube,
				fmt.Sprintf("ovnkube-node restart Δ=%d during window", n.OVNKube.RestartsDelta),
				"Restart count increased vs baseline — significant even if currently Ready.",
				Evidence{Label: "restarts", Current: fmt.Sprintf("%d", n.OVNKube.Restarts), Delta: fmt.Sprintf("+%d", n.OVNKube.RestartsDelta)})
		}
		if n.OVNKube.CrashLoop {
			add(RuleOVNKubeCrashLoop, SevCritical, CatOVNKube, "ovnkube-node CrashLoopBackOff",
				"Container waiting in CrashLoopBackOff.")
		}
		if n.OVNKube.OOMKilled {
			add(RuleOVNKubeOOM, SevCritical, CatOVNKube, "ovnkube-node OOMKilled",
				"Last termination reason OOMKilled.")
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
			// Informational log classes must not flip overall to DEGRADED.
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
		return "All observed nodes and ovnkube-node pods look healthy vs baseline. Confidence: HIGH"
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("Overall %s — %d healthy, %d warning, %d critical.",
		s.OverallState, s.HealthyCount, s.WarningCount, s.CriticalCount))
	n := 0
	conf := "MEDIUM"
	crits := 0
	for _, f := range s.Findings {
		if f.Severity == SevCritical || f.Severity == SevError {
			crits++
		}
	}
	if crits >= 2 || s.CriticalCount > 0 {
		conf = "HIGH"
	}
	for _, f := range s.Findings {
		if f.Severity == SevWarning || f.Severity == SevError || f.Severity == SevCritical {
			line := fmt.Sprintf("%s on %s: %s", f.RuleID, f.Node, f.Summary)
			if f.Why != "" {
				line += " — " + f.Why
			}
			parts = append(parts, line)
			n++
			if n >= 6 {
				break
			}
		}
	}
	parts = append(parts, "Confidence: "+conf)
	return strings.Join(parts, " ")
}
