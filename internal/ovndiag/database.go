package ovndiag

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// DatabaseLayer tracks OVN DB / northd container health on an ovnkube-node pod.
type DatabaseLayer struct {
	NBDBReady    bool `json:"nbdbReady"`
	SBDBReady    bool `json:"sbdbReady"`
	NorthdReady  bool `json:"northdReady"`
	ControllerOK bool `json:"controllerOK"`
	NBDBRestarts int  `json:"nbdbRestarts"`
	SBDBRestarts int  `json:"sbdbRestarts"`
	Present      bool `json:"present"` // false if containers not found (IC / different model)
}

func readDatabaseLayer(p corev1.Pod) DatabaseLayer {
	l := DatabaseLayer{}
	byName := map[string]corev1.ContainerStatus{}
	for _, cs := range p.Status.ContainerStatuses {
		byName[cs.Name] = cs
	}
	readOne := func(name string) (ready bool, restarts int, found bool) {
		cs, ok := byName[name]
		if !ok {
			return false, 0, false
		}
		return cs.Ready, int(cs.RestartCount), true
	}
	var found int
	if r, n, ok := readOne("nbdb"); ok {
		l.NBDBReady = r
		l.NBDBRestarts = n
		found++
	}
	if r, n, ok := readOne("sbdb"); ok {
		l.SBDBReady = r
		l.SBDBRestarts = n
		found++
	}
	if r, _, ok := readOne("northd"); ok {
		l.NorthdReady = r
		found++
	}
	if r, _, ok := readOne("ovn-controller"); ok {
		l.ControllerOK = r
		found++
	}
	l.Present = found > 0
	return l
}

func evaluateDatabase(n OVNNodeHealth, now time.Time, batchID int) []Finding {
	var out []Finding
	d := n.Database
	if !d.Present {
		return out
	}
	add := func(rule string, sev Severity, summary, why string, ev ...Evidence) {
		out = append(out, Finding{
			ID:     fmt.Sprintf("%s-%s-%d", rule, n.NodeName, now.Unix()),
			RuleID: rule, Severity: sev, Category: CatDatabase,
			Node: n.NodeName, Component: "ovn-db", FirstSeen: now, LastSeen: now, Count: 1,
			Summary: summary, Evidence: ev, BatchID: batchID, Why: why,
		})
	}
	if !d.NBDBReady {
		add(RuleOVNDBNotReady, SevError, "nbdb container not Ready",
			"Northbound DB container is not Ready — control-plane programming stalls.")
	}
	if !d.SBDBReady {
		add(RuleOVNDBNotReady, SevError, "sbdb container not Ready",
			"Southbound DB container is not Ready — ovn-controller may lose connectivity.")
	}
	if !d.NorthdReady {
		add(RuleOVNDBNotReady, SevWarning, "northd container not Ready",
			"northd translates NB→SB; not Ready means stale SB state.")
	}
	if !d.ControllerOK {
		add(RuleOVSProcessFail, SevError, "ovn-controller container not Ready",
			"ovn-controller programs OVS; not Ready is dataplane risk on this node.")
	}
	return out
}

func controlPlaneFindings(pods []corev1.Pod, now time.Time, batchID int) []Finding {
	var out []Finding
	for _, p := range pods {
		if !strings.Contains(p.Name, "ovnkube-control-plane") && !strings.Contains(p.Name, "ovnkube-master") {
			continue
		}
		ready := false
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if !ready || p.Status.Phase != corev1.PodRunning {
			out = append(out, Finding{
				ID:     fmt.Sprintf("%s-%s-%d", RuleOVNKubeNotReady, p.Name, now.Unix()),
				RuleID: RuleOVNKubeNotReady, Severity: SevCritical, Category: CatOVNKube,
				Node: p.Spec.NodeName, Component: p.Name, FirstSeen: now, LastSeen: now, Count: 1,
				Summary: fmt.Sprintf("control-plane pod %s not Ready (phase=%s)", p.Name, p.Status.Phase),
				BatchID: batchID,
				Why:     "OVN control-plane unavailability affects the whole cluster, not a single worker.",
			})
		}
	}
	return out
}
