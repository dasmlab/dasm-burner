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
	Cluster    string   `json:"cluster,omitempty"`
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

func (s *Server) cleanupCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Template string `json:"template"`
		Reason   string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Template == "" {
		body.Template = s.activeTemplateName()
	}
	cluster := s.currentCluster().Name
	sink := s.ensureLogSink()
	reason := body.Reason
	if reason == "" {
		reason = "manual refresh / check state"
	}
	sink.appendLog("info", "STATE", 0, fmt.Sprintf("check on cluster=%s template=%s (%s)", cluster, body.Template, reason))

	status, err := s.computeCleanupStatus(r, body.Template)
	if err != nil {
		sink.appendLog("error", "STATE", 0, "check failed: "+err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if status.Template != nil {
		t := status.Template
		if t.Deployed {
			sink.appendLog("warn", "STATE", 0, fmt.Sprintf("ONLINE on %s · %s · %d NS · %v",
				cluster, orEmpty(t.Prefix, "kb-?"), t.Count, t.Namespaces))
		} else {
			sink.appendLog("info", "STATE", 0, fmt.Sprintf("%s on %s · no managed NS for this template", t.Label, cluster))
		}
	}
	sink.appendLog("info", "STATE", 0, fmt.Sprintf("cluster has %d managed NS total across %d run(s)",
		status.ManagedTotal, len(status.LiveRuns)))
	for _, row := range status.LiveRuns {
		sink.appendLog("info", "STATE", 0, fmt.Sprintf("  live %s (%s) · %d NS", row.Prefix, orEmpty(row.Template, "?"), row.Count))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"template":     status.Template,
		"liveRuns":     status.LiveRuns,
		"managedTotal": status.ManagedTotal,
		"cluster":      cluster,
		"run":          sink.snapshot(),
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

type cleanupStatusPayload struct {
	Template     *deployStatus
	LiveRuns     []liveRunRow
	ManagedTotal int
}

type liveRunRow struct {
	RunID      string   `json:"runId"`
	Prefix     string   `json:"prefix"`
	Template   string   `json:"template,omitempty"`
	Count      int      `json:"count"`
	Namespaces []string `json:"namespaces,omitempty"`
}

func (s *Server) cleanupStatus(w http.ResponseWriter, r *http.Request) {
	template := strings.TrimSpace(r.URL.Query().Get("template"))
	status, err := s.computeCleanupStatus(r, template)
	if err != nil {
		if strings.Contains(err.Error(), "cluster") || strings.Contains(err.Error(), "kubeconfig") || strings.Contains(err.Error(), "config") {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"template":     status.Template,
		"liveRuns":     status.LiveRuns,
		"managedTotal": status.ManagedTotal,
		"cluster":      s.currentCluster().Name,
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) computeCleanupStatus(r *http.Request, template string) (*cleanupStatusPayload, error) {
	cl, err := s.liveClient(20, 40)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	allNS, err := cl.ListManagedNamespaces(ctx, "")
	if err != nil {
		return nil, err
	}
	byRun := map[string][]string{}
	for _, name := range allNS {
		rid := runIDFromNS(name)
		byRun[rid] = append(byRun[rid], name)
	}

	out := &cleanupStatusPayload{ManagedTotal: len(allNS)}
	cluster := s.currentCluster().Name

	if template != "" {
		st := &deployStatus{Template: template, Label: "cleaned", Deployed: false, Cluster: cluster}
		var matched []string
		var matchedRun string
		for _, e := range s.runsForTemplate(template) {
			if ns, ok := byRun[e.RunID]; ok && len(ns) > 0 {
				matched = append(matched, ns...)
				if matchedRun == "" {
					matchedRun = e.RunID
					st.Prefix = e.Prefix
					if st.Prefix == "" {
						st.Prefix = prefixForRun(e.RunID)
					}
				}
			}
		}
		last := s.lastRealRun(template)
		if last != nil && st.RunID == "" {
			st.RunID = last.RunID
			if st.Prefix == "" {
				st.Prefix = last.Prefix
			}
		}
		if matchedRun != "" {
			st.RunID = matchedRun
		}
		if len(matched) > 0 {
			sort.Strings(matched)
			st.Deployed = true
			st.Namespaces = matched
			st.Count = len(matched)
			st.Label = "online"
		} else if last == nil {
			st.Label = "unknown"
		}
		out.Template = st
	}

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
		out.LiveRuns = append(out.LiveRuns, liveRunRow{
			RunID: rid, Prefix: prefixForRun(rid), Template: tmpl,
			Count: len(ns), Namespaces: ns,
		})
	}
	sort.Slice(out.LiveRuns, func(i, j int) bool { return out.LiveRuns[i].RunID > out.LiveRuns[j].RunID })
	return out, nil
}

func runIDFromNS(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) >= 3 && parts[0] == "kb" {
		return parts[1]
	}
	return ""
}

func orEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func (s *Server) ensureLogSink() *execRun {
	m := s.execMgr()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		now := time.Now()
		m.cur = &execRun{
			ID:      fmt.Sprintf("session-%d", now.Unix()),
			Status:  "idle",
			Cluster: s.currentCluster().Name,
			Started: &now,
			Warning: "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		}
	}
	return m.cur
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

	m := s.execMgr()
	m.mu.Lock()
	if m.cur != nil && m.cur.Status == "running" {
		m.mu.Unlock()
		writeError(w, http.StatusConflict, fmt.Errorf("cannot cleanup while a test run is in progress"))
		return
	}
	m.mu.Unlock()

	cl, err := s.liveClient(20, 40)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	ctx := r.Context()
	cluster := s.currentCluster().Name
	sink := s.ensureLogSink()
	logf := func(level, msg string) {
		sink.appendLog(level, "CLEANUP", 0, msg)
	}

	var runIDs []string
	switch body.Scope {
	case "last":
		last := s.lastRealRun(body.Template)
		if last == nil {
			m := s.execMgr()
			m.mu.Lock()
			cur := m.cur
			m.mu.Unlock()
			if cur != nil && !cur.DryRun && cur.RunID != "" {
				runIDs = []string{cur.RunID}
			} else {
				logf("error", "no previous real run to clean")
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
			logf("error", fmt.Sprintf("no recorded runs for template %q", body.Template))
			writeError(w, http.StatusBadRequest, fmt.Errorf("no recorded runs for template %q", body.Template))
			return
		}
	case "all":
		runIDs = []string{""} // empty = all managed
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("scope must be last, template, or all"))
		return
	}

	logf("info", fmt.Sprintf("start scope=%s template=%s cluster=%s dryRun=%v wait=%v",
		body.Scope, orEmpty(body.Template, "—"), cluster, body.DryRun, body.Wait))
	if body.Scope == "all" {
		logf("warn", "targeting ALL managed namespaces (any run / any template) on this cluster")
	} else {
		for _, rid := range runIDs {
			logf("info", fmt.Sprintf("target run %s (%s)", rid, prefixForRun(rid)))
		}
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
	hadErr := false
	for _, rid := range runIDs {
		res, err := runner.Cleanup(ctx, runner.CleanupOptions{
			Cluster:     cl,
			RunID:       rid,
			DryRun:      body.DryRun,
			Wait:        body.Wait && !body.DryRun,
			WaitTimeout: 10 * time.Minute,
			Log: func(msg string) {
				logf("info", msg)
			},
		})
		row := result{RunID: rid, Prefix: prefixForRun(rid)}
		if res != nil {
			row.Namespaces = res.Namespaces
			row.Remaining = res.Remaining
		}
		if err != nil {
			hadErr = true
			row.Error = err.Error()
			logf("error", err.Error())
		}
		results = append(results, row)
	}

	dur := time.Since(started)
	deleted := 0
	for _, row := range results {
		deleted += len(row.Namespaces)
	}
	if hadErr {
		logf("error", fmt.Sprintf("finished with errors in %s · attempted %d NS", dur, deleted))
	} else {
		logf("info", fmt.Sprintf("done in %s · %d namespace(s)", dur, deleted))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"scope":    body.Scope,
		"dryRun":   body.DryRun,
		"cluster":  cluster,
		"results":  results,
		"duration": dur.String(),
		"selector": topology.Selector(""),
		"run":      sink.snapshot(),
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}
