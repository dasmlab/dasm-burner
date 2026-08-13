package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
)

func TestFreezeAndListSnapshots(t *testing.T) {
	dir := t.TempDir()
	apply := &runner.Report{
		RunID:       "6a98",
		Mode:        "batch",
		Started:     time.Now().Add(-time.Minute),
		Finished:    time.Now(),
		Duration:    time.Minute,
		Convergence: kube.Convergence{Overall: 100},
		Health: kube.Health{
			SampledAt:    time.Now(),
			NodesReady:   3,
			ManagedPods:  18,
			ManagedReady: 18,
			OVNPods:      6,
			OVNReady:     6,
		},
	}
	open := kube.Health{SampledAt: time.Now().Add(-time.Minute), NodesReady: 3, OVNPods: 6, OVNReady: 6}
	doc, err := Freeze(apply, "", Meta{
		Template: "smoke",
		Cluster:  "prod-1",
		Prefix:   "kb-6a98",
		Status:   "passed",
		Started:  apply.Started,
		Finished: apply.Finished,
		Desired:  DesiredCounts{Namespaces: 3, Pods: 18, Services: 6, Routes: 6, Deployments: 6},
		Open:     open,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Immutable || doc.SnapshotID == "" || doc.Open.Headline == "" || doc.Close.Headline == "" {
		t.Fatalf("%+v", doc)
	}
	id, err := WriteSnapshot(dir, doc, apply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "reports", id, "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.json")); err != nil {
		t.Fatal(err)
	}
	list, err := ListSnapshots(dir)
	if err != nil || len(list) != 1 || list[0].SnapshotID != id {
		t.Fatalf("%v %+v", err, list)
	}
	got, err := LoadSnapshot(dir, id)
	if err != nil || got.RunID != "6a98" || !got.Immutable {
		t.Fatalf("%v %+v", err, got)
	}
	// Ensure cleanup-sensitive fields are frozen copies, not empty
	if got.Close.Health == nil || got.Close.Health.ManagedReady != 18 {
		t.Fatalf("close health: %+v", got.Close.Health)
	}
}
