package etcddiag

import "fmt"

// CompareBaseline attaches baseline RSS and emits delta findings.
// Historical restart scars (ETCD004 on a quiet cluster) are ignored unless
// the count rose since baseline.
func CompareBaseline(snap, base *Snapshot) {
	if snap == nil || base == nil {
		Classify(snap)
		return
	}
	snap.BaselineAt = base.GeneratedAt
	snap.BaselineAPIRSSMi = base.APIRSSMi
	snap.BaselineEtcdRSSMi = base.EtcdRSSMi
	snap.BaselineOVNRSSMi = base.OVNRSSMi

	baseEtcdRC := map[string]int{}
	baseAPIRC := map[string]int{}
	for _, m := range base.Masters {
		baseEtcdRC[m.Name] = m.EtcdRestarts
		baseAPIRC[m.Name] = m.APIServerRestarts
	}
	for _, m := range snap.Masters {
		if prev, ok := baseEtcdRC[m.Name]; ok && m.EtcdRestarts > prev {
			addFinding(snap, "ETCD004", SevWarning, m.Name, "etcd",
				fmt.Sprintf("etcd on %s restarts %d→%d", m.Name, prev, m.EtcdRestarts),
				"Restart count rose during this run — fsync/disk or OOM on the member.")
		}
		if prev, ok := baseAPIRC[m.Name]; ok && m.APIServerRestarts > prev {
			addFinding(snap, "ETCD010", SevWarning, m.Name, "kube-apiserver",
				fmt.Sprintf("kube-apiserver on %s restarts %d→%d", m.Name, prev, m.APIServerRestarts),
				"API static pod restarted — RSS may drop then climb again with LIST/WATCH.")
		}
	}

	if base.APIRSSMi > 0 && snap.APIRSSMi > base.APIRSSMi*1.35 && snap.APIRSSMi-base.APIRSSMi > 800 {
		addFinding(snap, "ETCD007", SevWarning, "", "kube-apiserver",
			fmt.Sprintf("kube-apiserver RSS %.0f Mi vs baseline %.0f Mi", snap.APIRSSMi, base.APIRSSMi),
			"API working set grew under density. This is stage api_flex — etcd timeouts usually follow.")
	}

	Classify(snap)

	if snap.Cascade == StageLeftover {
		addFinding(snap, "ETCD008", SevWarning, "", "kube-apiserver",
			fmt.Sprintf("leftover API RSS %.0f Mi with workload pods=%d (baseline %.0f Mi)",
				snap.APIRSSMi, snap.WorkloadPods, base.APIRSSMi),
			"Deletes finished; Go RSS did not return to baseline. Recovers on kube-apiserver static-pod restart, not by waiting.")
	}
	if snap.Cascade == StageEtcdFlex || snap.Cascade == StageCollapse {
		addFinding(snap, "ETCD009", SevError, "", "cascade",
			fmt.Sprintf("cascade=%s", snap.Cascade),
			snap.CascadeWhy)
	}
	score(snap)
}

// Classify sets Cascade from live signals. Order we reproduce:
// idle → api_flex → etcd_flex → collapse, then leftover after cleanup.
func Classify(snap *Snapshot) {
	if snap == nil {
		return
	}
	apiDown := snap.APITotal > 0 && snap.APIReady < snap.APITotal
	etcdDown := snap.EtcdTotal > 0 && snap.EtcdReady < snap.EtcdTotal
	masterDown := snap.MastersTotal > 0 && snap.MastersReady < snap.MastersTotal
	quorum := snap.EtcdTotal >= 3 && snap.EtcdReady < (snap.EtcdTotal/2)+1
	apiFat := snap.BaselineAPIRSSMi > 0 && snap.APIRSSMi > snap.BaselineAPIRSSMi*1.35 && snap.APIRSSMi-snap.BaselineAPIRSSMi > 800
	// Labeled namespaces gone (including force-finalize). Ghost pods do not keep us in api_flex.
	empty := snap.WorkloadNS < 20

	switch {
	case masterDown || quorum:
		snap.Cascade = StageCollapse
		snap.CascadeWhy = "masters NotReady and/or etcd lost quorum — stop the burn."
	case etcdDown || snap.MemPressure > 0:
		snap.Cascade = StageEtcdFlex
		snap.CascadeWhy = "etcd members or master MemoryPressure — after API flex, before node collapse."
	case empty && snap.BaselineAPIRSSMi > 0 && snap.APIRSSMi > snap.BaselineAPIRSSMi+800:
		snap.Cascade = StageLeftover
		snap.CascadeWhy = "workload gone; API RSS still elevated vs baseline (ratchet)."
	case apiDown || apiFat:
		snap.Cascade = StageAPIFlex
		snap.CascadeWhy = "kube-apiserver RSS or Ready flexed first. LIST/WATCH cache growth precedes etcd timeouts."
	default:
		snap.Cascade = StageIdle
		snap.CascadeWhy = "control-plane Ready; no RSS climb vs baseline."
	}
}
