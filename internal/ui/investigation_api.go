package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/investigation"
)

func (s *Server) investigationsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := investigation.List(s.RunDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"investigations": list,
			"warning":        "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
			"note":           "Catalog items ship in git. PVC overlays status, notes, and evidence.",
		})
	case http.MethodPost:
		s.createInvestigation(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) investigationByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/investigations/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "evidence" {
		s.investigationEvidenceAPI(w, r, id)
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		inv, err := investigation.Get(s.RunDir, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"investigation": inv})
	case http.MethodPut:
		s.updateInvestigation(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createInvestigation(w http.ResponseWriter, r *http.Request) {
	var inv investigation.Investigation
	if err := json.NewDecoder(r.Body).Decode(&inv); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if inv.Title == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("title required"))
		return
	}
	if inv.ID == "" {
		inv.ID = inv.Title
	}
	inv.ID = investigation.IDFrom(inv.ID)
	if inv.Protocol == "" {
		inv.Protocol = "isolated-wave"
	}
	if inv.Cluster == "" {
		inv.Cluster = s.currentCluster().Name
	}
	inv.Catalog = false
	if err := investigation.Save(s.RunDir, inv); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := investigation.Get(s.RunDir, inv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"investigation": got})
}

func (s *Server) updateInvestigation(w http.ResponseWriter, r *http.Request, id string) {
	cur, err := investigation.Get(s.RunDir, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var over investigation.Investigation
	if err := json.NewDecoder(r.Body).Decode(&over); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if over.Status != "" {
		cur.Status = over.Status
	}
	if over.Title != "" {
		cur.Title = over.Title
	}
	if over.Hypothesis != "" {
		cur.Hypothesis = over.Hypothesis
	}
	if over.Metric != "" {
		cur.Metric = over.Metric
	}
	if over.Protocol != "" {
		cur.Protocol = over.Protocol
	}
	if over.Notes != "" {
		cur.Notes = over.Notes
	}
	if over.Cluster != "" {
		cur.Cluster = over.Cluster
	}
	if len(over.Pieces) > 0 {
		cur.Pieces = over.Pieces
	}
	if len(over.TestPlan) > 0 {
		cur.TestPlan = over.TestPlan
	}
	if len(over.SourceFiles) > 0 {
		cur.SourceFiles = over.SourceFiles
	}
	if over.PossibleFix != nil {
		cur.PossibleFix = over.PossibleFix
	}
	if len(over.Evidence) > 0 {
		cur.Evidence = over.Evidence
	}
	if err := investigation.Save(s.RunDir, *cur); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := investigation.Get(s.RunDir, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"investigation": got})
}

func (s *Server) investigationEvidenceAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var ev investigation.Evidence
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(ev.Note) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("note required"))
		return
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	if ev.Cluster == "" {
		ev.Cluster = s.currentCluster().Name
	}
	got, err := investigation.AppendEvidence(s.RunDir, id, ev)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"investigation": got})
}
