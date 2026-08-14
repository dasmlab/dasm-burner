package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteListCleanupReport(t *testing.T) {
	dir := t.TempDir()
	started := time.Now().Add(-2 * time.Minute)
	finished := time.Now()
	doc := &CleanupReport{
		Scope:      "all",
		Template:   "smoke500",
		Cluster:    "test-ovn-perf",
		Status:     "passed",
		Started:    started,
		Finished:   finished,
		RunIDs:     []string{"6a98"},
		Targeted:   CleanupObjectTotals{Namespaces: 10, Services: 20, Pods: 60},
		DeletedNS:  10,
		Namespaces: []string{"kb-6a98-ns-00001-xxxx"},
		Logs: []CleanupLogLine{
			{At: started, Level: "info", Message: "start"},
			{At: finished, Level: "info", Message: "done"},
		},
	}
	id, err := WriteCleanupReport(dir, doc)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	if _, err := os.Stat(filepath.Join(dir, "cleanup-reports", id, "report.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cleanup-reports", id, "summary.json")); err != nil {
		t.Fatal(err)
	}
	list, err := ListCleanupReports(dir)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if list[0].DurationMs <= 0 {
		t.Fatalf("expected positive duration, got %d", list[0].DurationMs)
	}
	got, err := LoadCleanupReport(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeletedNS != 10 || len(got.Logs) != 2 {
		t.Fatalf("got %+v", got)
	}
}
