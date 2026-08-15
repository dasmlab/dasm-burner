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
	if snap.Kind == "baseline" {
		_ = os.WriteFile(filepath.Join(runDir, "etcddiag", "baseline.json"), b, 0o644)
	}
	_ = appendSeries(runDir, snap, id)
	return id, nil
}

func LoadBaseline(runDir string) (*Snapshot, error) {
	b, err := os.ReadFile(filepath.Join(runDir, "etcddiag", "baseline.json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func appendSeries(runDir string, snap *Snapshot, id string) error {
	pt := SeriesPoint{
		At: snap.GeneratedAt, ID: id, Kind: snap.Kind, BatchID: snap.BatchID, RunID: snap.RunID,
		Cascade: snap.Cascade, WorkloadPods: snap.WorkloadPods, WorkloadNS: snap.WorkloadNS,
		APIRSSMi: snap.APIRSSMi, EtcdRSSMi: snap.EtcdRSSMi, OVNRSSMi: snap.OVNRSSMi,
		APIReady: snap.APIReady, APITotal: snap.APITotal,
		EtcdReady: snap.EtcdReady, EtcdTotal: snap.EtcdTotal,
		MastersReady: snap.MastersReady, MastersTotal: snap.MastersTotal,
	}
	path := filepath.Join(runDir, "etcddiag", "series.json")
	var pts []SeriesPoint
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &pts)
	}
	pts = append(pts, pt)
	if len(pts) > 200 {
		pts = pts[len(pts)-200:]
	}
	out, err := json.MarshalIndent(pts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func LoadSeries(runDir string) []SeriesPoint {
	b, err := os.ReadFile(filepath.Join(runDir, "etcddiag", "series.json"))
	if err != nil {
		return nil
	}
	var pts []SeriesPoint
	_ = json.Unmarshal(b, &pts)
	return pts
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
	Cascade       string      `json:"cascade,omitempty"`
	FindingCount  int         `json:"findingCount"`
	CriticalCount int         `json:"criticalCount"`
	MastersReady  int         `json:"mastersReady"`
	MastersTotal  int         `json:"mastersTotal"`
	EtcdReady     int         `json:"etcdReady"`
	EtcdTotal     int         `json:"etcdTotal"`
	WorkloadPods  int         `json:"workloadPods"`
	APIRSSMi      float64     `json:"apiserverRssMi,omitempty"`
	EtcdRSSMi     float64     `json:"etcdRssMi,omitempty"`
	OVNRSSMi      float64     `json:"ovnRssMi,omitempty"`
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
			OverallState: snap.OverallState, Cascade: snap.Cascade, FindingCount: snap.FindingCount,
			CriticalCount: snap.CriticalCount, MastersReady: snap.MastersReady, MastersTotal: snap.MastersTotal,
			EtcdReady: snap.EtcdReady, EtcdTotal: snap.EtcdTotal, WorkloadPods: snap.WorkloadPods,
			APIRSSMi: snap.APIRSSMi, EtcdRSSMi: snap.EtcdRSSMi, OVNRSSMi: snap.OVNRSSMi, Kind: snap.Kind,
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
