package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/report"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := report.ListSnapshots(s.RunDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reports": list,
		"count":   len(list),
		"note":    "Immutable end-of-run snapshots. Cleanup does not rewrite these.",
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) reportByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/reports/")
	id = strings.Trim(id, "/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	doc, err := report.LoadSnapshot(s.RunDir, id)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// report serves the latest immutable snapshot. Query ?id= selects a specific one.
// Live cluster health is never used for historical reports.
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		doc, err := report.LoadSnapshot(s.RunDir, id)
		if err != nil {
			if os.IsNotExist(err) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, doc)
		return
	}
	if doc, err := report.LatestSnapshot(s.RunDir); err == nil && doc != nil {
		writeJSON(w, http.StatusOK, doc)
		return
	}
	// Legacy single report.json (may itself be an immutable freeze).
	path := filepath.Join(s.RunDir, "report.json")
	if b, err := os.ReadFile(path); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"narrative": "No immutable report yet. Execute a run (or CLI apply + report) to freeze a snapshot.",
		"reports":   []any{},
		"warning":   "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func desiredFromGraph(g *topology.Graph) report.DesiredCounts {
	if g == nil {
		return report.DesiredCounts{}
	}
	c := g.Counts
	return report.DesiredCounts{
		Namespaces:  c.Namespaces,
		Services:    c.Services,
		Routes:      c.Routes,
		Deployments: c.Deployments,
		Pods:        c.Pods,
	}
}

func (s *Server) persistRunSnapshot(run *execRun, g *topology.Graph, apply *runner.Report, open kube.Health, status string, dryRun bool) (string, error) {
	if apply == nil {
		return "", nil
	}
	fin := time.Now()
	started := fin
	tmpl, cluster, pfx, rid := "", "", "", ""
	if run != nil {
		run.mu.Lock()
		tmpl = run.Template
		cluster = run.Cluster
		pfx = run.Prefix
		rid = run.RunID
		if run.Started != nil {
			started = *run.Started
		}
		if run.Finished != nil {
			fin = *run.Finished
		}
		run.mu.Unlock()
	}
	if apply != nil && !apply.Finished.IsZero() {
		fin = apply.Finished
	}
	if apply != nil && !apply.Started.IsZero() {
		started = apply.Started
	}
	if pfx == "" && rid != "" {
		pfx = prefixForRun(rid)
	}
	if pfx == "" && apply != nil && apply.RunID != "" {
		pfx = prefixForRun(apply.RunID)
	}
	meta := report.Meta{
		Template: tmpl,
		Cluster:  cluster,
		Prefix:   pfx,
		Status:   status,
		DryRun:   dryRun,
		Started:  started,
		Finished: fin,
		Desired:  desiredFromGraph(g),
		Open:     open,
	}
	doc, err := report.Freeze(apply, filepath.Join(s.RunDir, "kube-burner", "collected"), meta)
	if err != nil {
		return "", err
	}
	return report.WriteSnapshot(s.RunDir, doc, apply)
}
