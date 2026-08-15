package etcddiag

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

// SampleLive interrogates control-plane + etcd + kube-apiserver + RSS (metrics.k8s.io).
func SampleLive(ctx context.Context, cs kubernetes.Interface, dyn dynamic.Interface, runID, cluster string, batchID int) (*Snapshot, error) {
	if cs == nil {
		return nil, fmt.Errorf("nil clientset")
	}
	now := time.Now()
	snap := &Snapshot{
		GeneratedAt: now,
		RunID:       runID,
		Cluster:     cluster,
		BatchID:     batchID,
		Kind:        "sample",
	}

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	masters := map[string]*MasterNode{}
	for _, n := range nodes.Items {
		if !isMaster(n) {
			continue
		}
		m := &MasterNode{
			Name:           n.Name,
			Ready:          condTrue(n, corev1.NodeReady),
			MemoryPressure: condTrue(n, corev1.NodeMemoryPressure),
			DiskPressure:   condTrue(n, corev1.NodeDiskPressure),
			PIDPressure:    condTrue(n, corev1.NodePIDPressure),
		}
		snap.MastersTotal++
		if m.Ready {
			snap.MastersReady++
		} else {
			addFinding(snap, "ETCD001", SevCritical, n.Name, "node",
				fmt.Sprintf("Control-plane node %s Ready=False", n.Name),
				"Kubelet stopped posting Ready — etcd/API on this member are at risk.")
		}
		if m.MemoryPressure {
			snap.MemPressure++
			addFinding(snap, "ETCD003", SevWarning, n.Name, "node",
				fmt.Sprintf("Master %s MemoryPressure=True", n.Name),
				"etcd and kube-apiserver share this node RAM — MemoryPressure precedes etcd timeouts.")
		}
		masters[n.Name] = m
		snap.Masters = append(snap.Masters, *m)
	}

	for _, n := range nodes.Items {
		if isMaster(n) {
			continue
		}
		if n.Labels != nil {
			if _, ok := n.Labels["node-role.kubernetes.io/infra"]; ok {
				continue
			}
		}
		if !condTrue(n, corev1.NodeReady) {
			continue
		}
		snap.WorkerNodes++
		if q, ok := n.Status.Allocatable[corev1.ResourcePods]; ok && !q.IsZero() {
			if v := int(q.Value()); v > snap.MaxPodsTypical {
				snap.MaxPodsTypical = v
			}
		}
	}

	etcdPods, _ := cs.CoreV1().Pods("openshift-etcd").List(ctx, metav1.ListOptions{})
	for _, p := range etcdPods.Items {
		if !strings.HasPrefix(p.Name, "etcd-") || strings.Contains(p.Name, "guard") {
			continue
		}
		ready := podReady(p)
		rc := restartCount(p)
		mem := EtcdMember{PodName: p.Name, Node: p.Spec.NodeName, Ready: ready, Restarts: rc, Phase: string(p.Status.Phase)}
		snap.Etcd = append(snap.Etcd, mem)
		snap.EtcdTotal++
		if ready {
			snap.EtcdReady++
		} else {
			addFinding(snap, "ETCD002", SevError, p.Spec.NodeName, "etcd",
				fmt.Sprintf("etcd pod %s not Ready", p.Name),
				"Member unavailable — watch quorum and API latency.")
		}
		if m, ok := masters[p.Spec.NodeName]; ok {
			m.EtcdPod = p.Name
			m.EtcdReady = ready
			m.EtcdRestarts = rc
		}
	}
	if snap.EtcdTotal >= 3 && snap.EtcdReady < (snap.EtcdTotal/2)+1 {
		addFinding(snap, "ETCD006", SevCritical, "", "etcd",
			fmt.Sprintf("etcd quorum risk Ready=%d/%d", snap.EtcdReady, snap.EtcdTotal),
			"Fewer than majority of etcd members Ready — stop creates immediately.")
	}

	kas, _ := cs.CoreV1().Pods("openshift-kube-apiserver").List(ctx, metav1.ListOptions{})
	for _, p := range kas.Items {
		if !strings.HasPrefix(p.Name, "kube-apiserver-") || strings.Contains(p.Name, "guard") {
			continue
		}
		ready := podReady(p)
		snap.APITotal++
		if ready {
			snap.APIReady++
		} else {
			addFinding(snap, "ETCD005", SevError, p.Spec.NodeName, "kube-apiserver",
				fmt.Sprintf("kube-apiserver %s not Ready", p.Name),
				"API member down — LIST/WATCH storms amplify etcd load on remaining members.")
		}
		if m, ok := masters[p.Spec.NodeName]; ok {
			m.APIServerPod = p.Name
			m.APIServerReady = ready
			m.APIServerRestarts = restartCount(p)
		}
	}

	ovn, _ := cs.CoreV1().Pods("openshift-ovn-kubernetes").List(ctx, metav1.ListOptions{})
	for _, p := range ovn.Items {
		if !strings.Contains(p.Name, "ovnkube-node") {
			continue
		}
		if m, ok := masters[p.Spec.NodeName]; ok {
			m.OVNPod = p.Name
		}
	}

	nsList, _ := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: topology.LabelManaged + "=true"})
	snap.WorkloadNS = len(nsList.Items)
	pods, _ := cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: topology.LabelManaged + "=true"})
	snap.WorkloadPods = len(pods.Items)

	metricsHit := false
	for _, m := range masters {
		if mi, ok := containerRSSMi(ctx, dyn, "openshift-kube-apiserver", m.APIServerPod, "kube-apiserver"); ok {
			m.APIRSSMi = mi
			snap.APIRSSMi += mi
			metricsHit = true
		}
		if mi, ok := containerRSSMi(ctx, dyn, "openshift-etcd", m.EtcdPod, "etcd"); ok {
			m.EtcdRSSMi = mi
			snap.EtcdRSSMi += mi
			metricsHit = true
		}
		if mi, ok := containerRSSMi(ctx, dyn, "openshift-ovn-kubernetes", m.OVNPod, "ovnkube-controller"); ok {
			m.OVNRSSMi = mi
			snap.OVNRSSMi += mi
			metricsHit = true
		}
	}
	snap.MetricsOK = metricsHit

	// refresh masters slice with etcd/kas/rss annotations
	for i := range snap.Masters {
		if m, ok := masters[snap.Masters[i].Name]; ok {
			snap.Masters[i] = *m
		}
	}

	Classify(snap)
	score(snap)
	return snap, nil
}

func score(snap *Snapshot) {
	snap.CriticalCount, snap.WarningCount, snap.HealthyCount = 0, 0, 0
	for _, f := range snap.Findings {
		switch f.Severity {
		case SevCritical:
			snap.CriticalCount++
		case SevWarning, SevError:
			snap.WarningCount++
		default:
			snap.HealthyCount++
		}
	}
	snap.FindingCount = len(snap.Findings)
	switch {
	case snap.CriticalCount > 0:
		snap.OverallState = StateCritical
	case snap.WarningCount > 0:
		snap.OverallState = StateWarning
	default:
		snap.OverallState = StateHealthy
		snap.HealthyCount = snap.MastersReady
	}
	var why []string
	if snap.Cascade != "" && snap.Cascade != StageIdle {
		why = append(why, fmt.Sprintf("cascade=%s — %s", snap.Cascade, snap.CascadeWhy))
	}
	for _, f := range snap.Findings {
		if f.Severity == SevCritical || f.Severity == SevError {
			why = append(why, f.Summary)
		}
	}
	if len(why) > 6 {
		why = why[:6]
	}
	snap.WhyLines = why
	snap.Why = strings.Join(why, "\n")
}

func addFinding(snap *Snapshot, rule string, sev Severity, node, comp, summary, why string) {
	snap.Findings = append(snap.Findings, Finding{
		ID:     fmt.Sprintf("%s-%d", rule, len(snap.Findings)+1),
		RuleID: rule, Severity: sev, Node: node, Component: comp,
		Summary: summary, Why: why, BatchID: snap.BatchID, At: snap.GeneratedAt,
	})
}

func isMaster(n corev1.Node) bool {
	if n.Labels == nil {
		return false
	}
	if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
		return true
	}
	_, ok := n.Labels["node-role.kubernetes.io/control-plane"]
	return ok
}

func condTrue(n corev1.Node, t corev1.NodeConditionType) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == t {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func restartCount(p corev1.Pod) int {
	n := 0
	for _, c := range p.Status.ContainerStatuses {
		n += int(c.RestartCount)
	}
	return n
}
