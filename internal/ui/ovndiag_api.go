package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	if !s.enqueueOVN("baseline", 0, s.ensureLogSink()) {
		writeError(w, http.StatusConflict, fmt.Errorf("OVN worker busy — wait for the in-flight sample"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"async":    true,
		"kind":     "baseline",
		"stream":   eventsPath,
		"message":  "baseline queued on OVN worker; Reload latest or watch SSE event ovn",
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
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
	if !s.enqueueOVN("sample", body.BatchID, s.ensureLogSink()) {
		writeError(w, http.StatusConflict, fmt.Errorf("OVN worker busy — wait for the in-flight sample"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"async":    true,
		"kind":     "sample",
		"stream":   eventsPath,
		"message":  "sample queued on OVN worker; Reload latest or watch SSE event ovn",
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
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
