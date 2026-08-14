package ovndiag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WriteSnapshot freezes a diagnoser Snapshot under {runDir}/ovndiag/{id}/snapshot.json
func WriteSnapshot(runDir string, snap *Snapshot) (string, error) {
	if snap == nil {
		return "", fmt.Errorf("nil snapshot")
	}
	id := fmt.Sprintf("%s-%d", orRun(snap.RunID), snap.GeneratedAt.Unix())
	dir := filepath.Join(runDir, "ovndiag", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), b, 0o644); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(runDir, "ovndiag", "latest.json"), b, 0o644)
	return id, nil
}

func LoadLatest(runDir string) (*Snapshot, error) {
	b, err := os.ReadFile(filepath.Join(runDir, "ovndiag", "latest.json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func List(runDir string) ([]string, error) {
	root := filepath.Join(runDir, "ovndiag")
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range ents {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	return ids, nil
}

// SnapshotSummary is a compact history row for the OVN Diagnoser UI.
type SnapshotSummary struct {
	ID            string      `json:"id"`
	GeneratedAt   time.Time   `json:"generatedAt"`
	BaselineAt    time.Time   `json:"baselineAt,omitempty"`
	RunID         string      `json:"runId,omitempty"`
	Cluster       string      `json:"cluster,omitempty"`
	OverallState  HealthState `json:"overallState"`
	HealthyCount  int         `json:"healthyCount"`
	WarningCount  int         `json:"warningCount"`
	CriticalCount int         `json:"criticalCount"`
	FindingCount  int         `json:"findingCount"`
	NodeCount     int         `json:"nodeCount"`
	Kind          string      `json:"kind"` // baseline | sample | watch
}

func LoadByID(runDir, id string) (*Snapshot, error) {
	b, err := os.ReadFile(filepath.Join(runDir, "ovndiag", id, "snapshot.json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func ListSummaries(runDir string, limit int) ([]SnapshotSummary, error) {
	ids, err := List(runDir)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 40
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]SnapshotSummary, 0, len(ids))
	for _, id := range ids {
		snap, err := LoadByID(runDir, id)
		if err != nil || snap == nil {
			continue
		}
		kind := "sample"
		if !snap.BaselineAt.IsZero() && snap.GeneratedAt.Sub(snap.BaselineAt) < 2*time.Second {
			kind = "baseline"
		}
		if strings.Contains(id, "watch") {
			kind = "watch"
		}
		warnN := 0
		for _, f := range snap.Findings {
			if f.Severity == SevWarning || f.Severity == SevError || f.Severity == SevCritical || f.Severity == SevNotice {
				warnN++
			}
		}
		out = append(out, SnapshotSummary{
			ID: id, GeneratedAt: snap.GeneratedAt, BaselineAt: snap.BaselineAt,
			RunID: snap.RunID, Cluster: snap.Cluster, OverallState: snap.OverallState,
			HealthyCount: snap.HealthyCount, WarningCount: snap.WarningCount, CriticalCount: snap.CriticalCount,
			FindingCount: warnN, NodeCount: len(snap.Nodes), Kind: kind,
		})
	}
	return out, nil
}

func orRun(s string) string {
	if s == "" {
		return "norun"
	}
	return s
}

// MarkBatch appends a batch lifecycle marker into latest timeline (best-effort).
func MarkBatch(runDir string, batchID int, summary string) error {
	snap, err := LoadLatest(runDir)
	if err != nil {
		snap = &Snapshot{GeneratedAt: time.Now(), OverallState: StateHealthy}
	}
	ev := TimelineEvent{At: time.Now(), Kind: "batch", Summary: summary, BatchID: batchID}
	snap.BatchMarkers = append(snap.BatchMarkers, ev)
	snap.Timeline = append(snap.Timeline, ev)
	_, err = WriteSnapshot(runDir, snap)
	return err
}
