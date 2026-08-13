package ui

import (
	"net/http"
	"strings"

	"github.com/dasmlab/dasm-burner/internal/report"
)

func (s *Server) cleanupReportsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := report.ListCleanupReports(s.RunDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reports": list,
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		"note":    "Immutable cleanup jobs (duration, targeted objects, live log).",
	})
}

func (s *Server) cleanupReportByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/cleanup-reports/")
	id = strings.Trim(id, "/")
	if id == "" || id == "latest" {
		doc, err := report.LatestCleanupReport(s.RunDir)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"report": doc})
		return
	}
	doc, err := report.LoadCleanupReport(s.RunDir, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": doc})
}
