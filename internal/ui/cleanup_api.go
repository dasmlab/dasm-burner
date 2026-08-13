package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

type deployStatus struct {
	Template   string   `json:"template,omitempty"`
	RunID      string   `json:"runId,omitempty"`
	Prefix     string   `json:"prefix,omitempty"`
	Deployed   bool     `json:"deployed"`
	Namespaces []string `json:"namespaces,omitempty"`
	Count      int      `json:"count"`
	Label      string   `json:"label"` // online | cleaned | unknown
}

func (s *Server) cleanupAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cleanupStatus(w, r)
	case http.MethodPost:
		s.doCleanup(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) cleanupStatus(w http.ResponseWriter, r *http.Request) {
	template := strings.TrimSpace(r.URL.Query().Get("template"))
	cl, err := s.liveClient(20, 40)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	ctx := r.Context()

	// All managed namespaces (any run).
	allNS, err := cl.ListManagedNamespaces(ctx, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	byRun := map[string][]string{}
	for _, name := range allNS {
		rid := runIDFromNS(name)
		byRun[rid] = append(byRun[rid], name)
	}

	var templateStatus *deployStatus
	if template != "" {
		last := s.lastRealRun(template)
		st := &deployStatus{Template: template, Label: "cleaned", Deployed: false}
		if last != nil {
			st.RunID = last.RunID
			st.Prefix = last.Prefix
			if ns, ok := byRun[last.RunID]; ok && len(ns) > 0 {
				st.Deployed = true
				st.Namespaces = ns
				st.Count = len(ns)
				st.Label = "online"
			}
		} else {
			// Infer from live NS matching history-less seed? leave cleaned.
			st.Label = "unknown"
		}
		templateStatus = st
	}

	type runRow struct {
		RunID      string   `json:"runId"`
		Prefix     string   `json:"prefix"`
		Template   string   `json:"template,omitempty"`
		Count      int      `json:"count"`
		Namespaces []string `json:"namespaces,omitempty"`
	}
	var live []runRow
	for rid, ns := range byRun {
		if rid == "" {
			continue
		}
		sort.Strings(ns)
		tmpl := ""
		for _, e := range s.runsForTemplate("") {
			if e.RunID == rid {
				tmpl = e.Template
				break
			}
		}
		live = append(live, runRow{
			RunID: rid, Prefix: prefixForRun(rid), Template: tmpl,
			Count: len(ns), Namespaces: ns,
		})
	}
	sort.Slice(live, func(i, j int) bool { return live[i].RunID > live[j].RunID })

	writeJSON(w, http.StatusOK, map[string]any{
		"template":     templateStatus,
		"liveRuns":     live,
		"managedTotal": len(allNS),
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func runIDFromNS(name string) string {
	// kb-{runID}-ns-{seq}-{sfx}
	parts := strings.Split(name, "-")
	if len(parts) >= 3 && parts[0] == "kb" {
		return parts[1]
	}
	return ""
}

func (s *Server) doCleanup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scope    string `json:"scope"` // last | template | all
		Template string `json:"template"`
		Wait     bool   `json:"wait"`
		DryRun   bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Scope == "" {
		body.Scope = "last"
	}
	cl, err := s.liveClient(20, 40)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	ctx := r.Context()

	var runIDs []string
	switch body.Scope {
	case "last":
		last := s.lastRealRun(body.Template)
		if last == nil {
			// fall back to current exec run
			m := s.execMgr()
			m.mu.Lock()
			cur := m.cur
			m.mu.Unlock()
			if cur != nil && !cur.DryRun && cur.RunID != "" {
				runIDs = []string{cur.RunID}
			} else {
				writeError(w, http.StatusBadRequest, fmt.Errorf("no previous real run to clean"))
				return
			}
		} else {
			runIDs = []string{last.RunID}
		}
	case "template":
		if body.Template == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("template required for scope=template"))
			return
		}
		for _, e := range s.runsForTemplate(body.Template) {
			runIDs = append(runIDs, e.RunID)
		}
		if len(runIDs) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("no recorded runs for template %q", body.Template))
			return
		}
	case "all":
		runIDs = []string{""} // empty = all managed
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("scope must be last, template, or all"))
		return
	}

	type result struct {
		RunID      string   `json:"runId"`
		Prefix     string   `json:"prefix,omitempty"`
		Namespaces []string `json:"namespaces"`
		Remaining  []string `json:"remaining,omitempty"`
		Error      string   `json:"error,omitempty"`
	}
	var results []result
	started := time.Now()
	for _, rid := range runIDs {
		res, err := runner.Cleanup(ctx, runner.CleanupOptions{
			Cluster:     cl,
			RunID:       rid,
			DryRun:      body.DryRun,
			Wait:        body.Wait && !body.DryRun,
			WaitTimeout: 10 * time.Minute,
		})
		row := result{RunID: rid, Prefix: prefixForRun(rid)}
		if res != nil {
			row.Namespaces = res.Namespaces
			row.Remaining = res.Remaining
		}
		if err != nil {
			row.Error = err.Error()
		}
		results = append(results, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"scope":    body.Scope,
		"dryRun":   body.DryRun,
		"results":  results,
		"duration": time.Since(started).String(),
		"selector": topology.Selector(""),
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}
