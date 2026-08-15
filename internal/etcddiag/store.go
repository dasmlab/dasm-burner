package etcddiag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func WriteSnapshot(runDir string, snap *Snapshot) (string, error) {
	if snap == nil {
		return "", fmt.Errorf("nil snapshot")
	}
	id := fmt.Sprintf("%s-%d", orRun(snap.RunID), snap.GeneratedAt.Unix())
	dir := filepath.Join(runDir, "etcddiag", id)
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
	_ = os.WriteFile(filepath.Join(runDir, "etcddiag", "latest.json"), b, 0o644)
	return id, nil
}

func LoadLatest(runDir string) (*Snapshot, error) {
	b, err := os.ReadFile(filepath.Join(runDir, "etcddiag", "latest.json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func LoadByID(runDir, id string) (*Snapshot, error) {
	b, err := os.ReadFile(filepath.Join(runDir, "etcddiag", id, "snapshot.json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

type SnapshotSummary struct {
	ID            string      `json:"id"`
	GeneratedAt   time.Time   `json:"generatedAt"`
	RunID         string      `json:"runId,omitempty"`
	Cluster       string      `json:"cluster,omitempty"`
	OverallState  HealthState `json:"overallState"`
	FindingCount  int         `json:"findingCount"`
	CriticalCount int         `json:"criticalCount"`
	MastersReady  int         `json:"mastersReady"`
	MastersTotal  int         `json:"mastersTotal"`
	EtcdReady     int         `json:"etcdReady"`
	EtcdTotal     int         `json:"etcdTotal"`
	Kind          string      `json:"kind"`
}

func ListSummaries(runDir string, limit int) ([]SnapshotSummary, error) {
	root := filepath.Join(runDir, "etcddiag")
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
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]SnapshotSummary, 0, len(ids))
	for _, id := range ids {
		snap, err := LoadByID(runDir, id)
		if err != nil {
			continue
		}
		out = append(out, SnapshotSummary{
			ID: id, GeneratedAt: snap.GeneratedAt, RunID: snap.RunID, Cluster: snap.Cluster,
			OverallState: snap.OverallState, FindingCount: snap.FindingCount,
			CriticalCount: snap.CriticalCount, MastersReady: snap.MastersReady, MastersTotal: snap.MastersTotal,
			EtcdReady: snap.EtcdReady, EtcdTotal: snap.EtcdTotal, Kind: snap.Kind,
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
