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
		Logs: []RunLogLine{
			{At: apply.Started, Level: "info", Phase: "PRECHECK", Message: "ok"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Immutable || doc.SnapshotID == "" || doc.Open.Headline == "" || doc.Close.Headline == "" {
		t.Fatalf("%+v", doc)
	}
	if doc.DurationMs <= 0 || doc.Duration == "" {
		t.Fatalf("expected wall duration, got duration=%q ms=%d", doc.Duration, doc.DurationMs)
	}
	if doc.ApplyDurationMs <= 0 || doc.BatchCount != 0 {
		// BatchCount 0 is fine when Batches empty; apply duration must be set
		if doc.ApplyDurationMs <= 0 {
			t.Fatalf("expected apply duration ms, got %d", doc.ApplyDurationMs)
		}
	}
	if len(doc.Logs) != 1 {
		t.Fatalf("expected frozen logs, got %d", len(doc.Logs))
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
	if _, err := os.Stat(filepath.Join(dir, "reports", id, "summary.json")); err != nil {
		t.Fatal(err)
	}
	list, err := ListSnapshots(dir)
	if err != nil || len(list) != 1 || list[0].SnapshotID != id {
		t.Fatalf("%v %+v", err, list)
	}
	if list[0].DurationMs <= 0 || list[0].Duration == "" {
		t.Fatalf("list missing duration: %+v", list[0])
	}
	got, err := LoadSnapshot(dir, id)
	if err != nil || got.RunID != "6a98" || !got.Immutable {
		t.Fatalf("%v %+v", err, got)
	}
	if got.Close.Health == nil || got.Close.Health.ManagedReady != 18 {
		t.Fatalf("close health: %+v", got.Close.Health)
	}
}
