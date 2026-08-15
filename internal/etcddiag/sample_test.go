package etcddiag

import (
	"testing"
	"time"
)

func TestScoreCritical(t *testing.T) {
	snap := &Snapshot{GeneratedAt: time.Now()}
	addFinding(snap, "ETCD001", SevCritical, "master-0", "node", "down", "why")
	score(snap)
	if snap.OverallState != StateCritical || snap.CriticalCount != 1 {
		t.Fatalf("%+v", snap)
	}
}

func TestRuleCatalog(t *testing.T) {
	if RuleCatalog["ETCD006"].ID == "" {
		t.Fatal("missing ETCD006")
	}
}
