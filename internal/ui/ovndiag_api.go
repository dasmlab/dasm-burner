package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/ovndiag"
)

func (s *Server) ovnBaseline() *ovndiag.Baseline {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ovnBase == nil {
		s.ovnBase = ovndiag.NewBaseline()
	}
	return s.ovnBase
}

func (s *Server) ovndiagAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.ovndiagGet(w, r)
	case http.MethodPost:
		s.ovndiagSample(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ovndiagHistoryAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/ovndiag/history")
	path = strings.Trim(path, "/")
	if path != "" {
		snap, err := ovndiag.LoadByID(s.RunDir, path)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("snapshot %s: %w", path, err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshot": snap,
			"rules":    ovndiag.RuleCatalog,
		})
		return
	}
	sums, err := ovndiag.ListSummaries(s.RunDir, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": sums,
		"rules":   ovndiag.RuleCatalog,
		"store":   "PVC /data (runDir/ovndiag/<id>/snapshot.json)",
	})
}

func (s *Server) ovndiagBaselineAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap, err := s.runOVNSample(r, 0)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	s.ovnBaseline().Capture(snap.Nodes)
	snap.BaselineAt = s.ovnBaseline().At()
	id, _ := ovndiag.WriteSnapshot(s.RunDir, snap)
	writeJSON(w, http.StatusOK, map[string]any{
		"baselineAt": snap.BaselineAt,
		"snapshotId": id,
		"snapshot":   snap,
		"captured":   baselineCaptureSummary(snap),
		"rules":      ovndiag.RuleCatalog,
		"warning":    "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) ovndiagGet(w http.ResponseWriter, r *http.Request) {
	sums, _ := ovndiag.ListSummaries(s.RunDir, 50)
	if snap, err := ovndiag.LoadLatest(s.RunDir); err == nil && snap != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshot": snap,
			"cached":   true,
			"samples":  sums,
			"rules":    ovndiag.RuleCatalog,
			"baseline": liveBaselineInfo(s),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": nil,
		"cached":   true,
		"samples":  sums,
		"rules":    ovndiag.RuleCatalog,
		"baseline": liveBaselineInfo(s),
	})
}

func (s *Server) ovndiagSample(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BatchID int `json:"batchId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	snap, err := s.runOVNSample(r, body.BatchID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	id, _ := ovndiag.WriteSnapshot(s.RunDir, snap)
	sums, _ := ovndiag.ListSummaries(s.RunDir, 50)
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshotId": id,
		"snapshot":   snap,
		"samples":    sums,
		"rules":      ovndiag.RuleCatalog,
		"baseline":   liveBaselineInfo(s),
		"warning":    "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) runOVNSample(r *http.Request, batchID int) (*ovndiag.Snapshot, error) {
	cl, err := s.liveClient(20, 40)
	if err != nil {
		return nil, err
	}
	live, ok := cl.(*kube.Live)
	if !ok || live.Clientset() == nil {
		return nil, fmt.Errorf("ovndiag requires a live cluster clientset")
	}
	runID := ""
	m := s.execMgr()
	m.mu.Lock()
	if m.cur != nil {
		runID = m.cur.RunID
	}
	m.mu.Unlock()
	return ovndiag.SampleLive(r.Context(), live.Clientset(), live.Dynamic(), s.ovnBaseline(), runID, s.currentCluster().Name, batchID)
}

func liveBaselineInfo(s *Server) map[string]any {
	b := s.ovnBaseline()
	at := b.At()
	out := map[string]any{
		"captured": !at.IsZero(),
		"at":       nil,
		"store":    "in-memory watermarks (restarts, Ready, CPU/mem) + snapshot on PVC under runDir/ovndiag/",
		"watermarks": []string{
			"per ovnkube-node restart count",
			"per-node Ready",
			"per-container CPU/mem (when metrics.k8s.io is available)",
		},
	}
	if !at.IsZero() {
		out["at"] = at
	}
	return out
}

func baselineCaptureSummary(snap *ovndiag.Snapshot) map[string]any {
	if snap == nil {
		return map[string]any{}
	}
	pods := 0
	for _, n := range snap.Nodes {
		if n.OVNKube.PodName != "" {
			pods++
		}
	}
	return map[string]any{
		"at":           snap.BaselineAt,
		"nodes":        len(snap.Nodes),
		"ovnkubePods":  pods,
		"overallState": snap.OverallState,
		"findingCount": len(snap.Findings),
		"what":         "Restart / Ready / resource watermarks for later Δ comparison",
		"where":        "Process memory (baseline) + PVC snapshot under /data/ovndiag/",
		"cluster":      snap.Cluster,
		"generatedAt":  snap.GeneratedAt.Format(time.RFC3339),
	}
}
