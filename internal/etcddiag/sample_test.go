package etcddiag

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassifyLeftoverAfterCleanup(t *testing.T) {
	snap := &Snapshot{
		APIRSSMi:         14000,
		BaselineAPIRSSMi: 8000,
		WorkloadPods:     0,
		WorkloadNS:       0,
		MastersReady:     3, MastersTotal: 3,
		EtcdReady: 3, EtcdTotal: 3,
		APIReady: 3, APITotal: 3,
	}
	Classify(snap)
	if snap.Cascade != StageLeftover {
		t.Fatalf("got %s want leftover", snap.Cascade)
	}
}

func TestClassifyLeftoverGhostPodsAfterForceFinalize(t *testing.T) {
	snap := &Snapshot{
		APIRSSMi:         10162,
		BaselineAPIRSSMi: 7846,
		WorkloadPods:     909,
		WorkloadNS:       0,
		MastersReady:     3, MastersTotal: 3,
		EtcdReady: 3, EtcdTotal: 3,
		APIReady: 3, APITotal: 3,
	}
	Classify(snap)
	if snap.Cascade != StageLeftover {
		t.Fatalf("got %s want leftover (NS gone, RSS still fat)", snap.Cascade)
	}
}

func TestClassifyAPIFlexBeforeEtcd(t *testing.T) {
	snap := &Snapshot{
		APIRSSMi:         12000,
		BaselineAPIRSSMi: 7000,
		WorkloadPods:     4000,
		WorkloadNS:       187,
		MastersReady:     3, MastersTotal: 3,
		EtcdReady: 3, EtcdTotal: 3,
		APIReady: 3, APITotal: 3,
	}
	Classify(snap)
	if snap.Cascade != StageAPIFlex {
		t.Fatalf("got %s want api_flex", snap.Cascade)
	}
}

func TestClassifyCollapse(t *testing.T) {
	snap := &Snapshot{MastersReady: 2, MastersTotal: 3, EtcdReady: 3, EtcdTotal: 3, APIReady: 3, APITotal: 3}
	Classify(snap)
	if snap.Cascade != StageCollapse {
		t.Fatalf("got %s", snap.Cascade)
	}
}

func TestCompareBaselineIgnoresOldRestarts(t *testing.T) {
	base := &Snapshot{
		GeneratedAt: time.Now().Add(-time.Hour),
		APIRSSMi:    8000,
		Masters:     []MasterNode{{Name: "m0", EtcdRestarts: 8, APIServerRestarts: 15}},
	}
	snap := &Snapshot{
		APIRSSMi:     8100,
		WorkloadPods: 0,
		MastersReady: 3, MastersTotal: 3,
		EtcdReady: 3, EtcdTotal: 3,
		APIReady: 3, APITotal: 3,
		Masters: []MasterNode{{Name: "m0", EtcdRestarts: 8, APIServerRestarts: 15}},
	}
	CompareBaseline(snap, base)
	for _, f := range snap.Findings {
		if f.RuleID == "ETCD004" || f.RuleID == "ETCD010" {
			t.Fatalf("scar restart should not fire: %+v", f)
		}
	}
}

func TestSeriesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	snap := &Snapshot{GeneratedAt: time.Now(), Kind: "baseline", APIRSSMi: 9000, Cascade: StageIdle, RunID: "t1"}
	id, err := WriteSnapshot(dir, snap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(dir); err != nil {
		t.Fatal(err)
	}
	pts := LoadSeries(dir)
	if len(pts) != 1 || pts[0].ID != id || pts[0].APIRSSMi != 9000 {
		t.Fatalf("%+v", pts)
	}
	if _, err := os.Stat(filepath.Join(dir, "etcddiag", "baseline.json")); err != nil {
		t.Fatal(err)
	}
}

func TestParseMemoryMiB(t *testing.T) {
	if g := parseMemoryMiB("5387Mi"); g < 5380 || g > 5390 {
		t.Fatalf("%v", g)
	}
	if g := parseMemoryMiB("5Gi"); g != 5120 {
		t.Fatalf("%v", g)
	}
}

func TestRuleCatalog(t *testing.T) {
	for _, id := range []string{"ETCD006", "ETCD007", "ETCD008", "ETCD009", "ETCD010"} {
		if RuleCatalog[id].ID == "" {
			t.Fatalf("missing %s", id)
		}
	}
}
