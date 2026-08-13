package ui

import (
	"encoding/json"
	"fmt"
	"net/http"

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
		"warning":    "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) ovndiagGet(w http.ResponseWriter, r *http.Request) {
	if snap, err := ovndiag.LoadLatest(s.RunDir); err == nil && snap != nil {
		writeJSON(w, http.StatusOK, map[string]any{"snapshot": snap, "cached": true})
		return
	}
	snap, err := s.runOVNSample(r, 0)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	_, _ = ovndiag.WriteSnapshot(s.RunDir, snap)
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": snap, "cached": false})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshotId": id,
		"snapshot":   snap,
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
