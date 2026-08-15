package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/cleanupwatch"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/report"
	"github.com/dasmlab/dasm-burner/internal/runner"
)

type deployStatus struct {
	Template   string        `json:"template,omitempty"`
	RunID      string        `json:"runId,omitempty"`
	Prefix     string        `json:"prefix,omitempty"`
	Deployed   bool          `json:"deployed"`
	Namespaces []string      `json:"namespaces,omitempty"`
	Count      int           `json:"count"`
	Label      string        `json:"label"` // online | cleaned | unknown
	Cluster    string        `json:"cluster,omitempty"`
	Objects    *objectCounts `json:"objects,omitempty"`
}

type objectCounts struct {
	Namespaces  int            `json:"namespaces"`
	Routes      int            `json:"routes"`
	Services    int            `json:"services"`
	Deployments int            `json:"deployments"`
	Pods        int            `json:"pods"`
	ReadyPods   int            `json:"readyPods"`
	PodPhases   map[string]int `json:"podPhases,omitempty"`
}

type liveRunRow struct {
	RunID      string        `json:"runId"`
	Prefix     string        `json:"prefix"`
	Template   string        `json:"template,omitempty"`
	Count      int           `json:"count"`
	Namespaces []string      `json:"namespaces,omitempty"`
	Objects    *objectCounts `json:"objects,omitempty"`
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

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	status, err := s.computeCleanupStatus(ctx, body.Template)
	if err != nil {
		sink.appendLog("error", "STATE", 0, "check failed: "+err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if status.Template != nil {
		t := status.Template
		if t.Deployed {
			sink.appendLog("warn", "STATE", 0, fmt.Sprintf("ONLINE for template %s on %s · %s · %d NS",
				t.Template, cluster, orEmpty(t.Prefix, "kb-?"), t.Count))
		} else if status.ManagedTotal > 0 {
			sink.appendLog("info", "STATE", 0, fmt.Sprintf(
				"template %s has no live NS on %s — cluster still has %d managed NS under other run(s)/templates",
				t.Template, cluster, status.ManagedTotal))
		} else {
			sink.appendLog("info", "STATE", 0, fmt.Sprintf("template %s clean on %s (no managed NS on cluster)", t.Template, cluster))
		}
	}
	sink.appendLog("info", "STATE", 0, fmt.Sprintf("cluster has %d managed NS total across %d run(s)",
		status.ManagedTotal, len(status.LiveRuns)))
	for _, row := range status.LiveRuns {
		sink.appendLog("info", "STATE", 0, fmt.Sprintf("  live %s (%s) · %d NS", row.Prefix, orEmpty(row.Template, "unlabeled"), row.Count))
	}
	s.publishCleanup(status)
	writeJSON(w, http.StatusOK, map[string]any{
		"template":     status.Template,
		"liveRuns":     status.LiveRuns,
		"managedTotal": status.ManagedTotal,
		"cluster":      cluster,
		"cleaning":     s.isCleanupBusy(),
		"run":          sink.snapshotSlim(),
		"stream":       eventsPath,
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

type cleanupStatusPayload struct {
	Template     *deployStatus
	LiveRuns     []liveRunRow
	ManagedTotal int
}

func (s *Server) cleanupStatus(w http.ResponseWriter, r *http.Request) {
	template := strings.TrimSpace(r.URL.Query().Get("template"))
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	status, err := s.computeCleanupStatus(ctx, template)
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
		"cleaning":     s.isCleanupBusy(),
		"stream":       eventsPath,
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

type nsMeta struct {
	name, runID, config string
}

func (s *Server) computeCleanupStatus(ctx context.Context, template string) (*cleanupStatusPayload, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	metas, cluster, err := s.managedMetas(ctx)
	if err != nil {
		return nil, err
	}

	byRun := map[string][]string{}
	configByRun := map[string]string{}
	for _, m := range metas {
		rid := m.runID
		if rid == "" {
			rid = runIDFromNS(m.name)
		}
		byRun[rid] = append(byRun[rid], m.name)
		if m.config != "" {
			if prev, ok := configByRun[rid]; !ok || prev == "" {
				configByRun[rid] = m.config
			}
		}
	}

	out := &cleanupStatusPayload{ManagedTotal: len(metas)}

	if template != "" {
		st := &deployStatus{Template: template, Label: "cleaned", Deployed: false, Cluster: cluster}
		var matched []string
		var matchedRun string

		for rid, ns := range byRun {
			if configByRun[rid] != template {
				continue
			}
			matched = append(matched, ns...)
			if matchedRun == "" {
				matchedRun = rid
				st.Prefix = prefixForRun(rid)
			}
		}
		if len(matched) == 0 {
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
			if len(matched) > 40 {
				st.Namespaces = append([]string(nil), matched[:40]...)
			} else {
				st.Namespaces = matched
			}
			st.Count = len(matched)
			st.Label = "online"
		} else {
			st.Label = "cleaned"
		}
		out.Template = st
	}

	histByRun := map[string]string{}
	for _, e := range s.runsForTemplate("") {
		if e.RunID != "" && e.Template != "" {
			histByRun[e.RunID] = e.Template
		}
	}
	for rid, ns := range byRun {
		if rid == "" {
			continue
		}
		sort.Strings(ns)
		tmpl := configByRun[rid]
		if tmpl == "" {
			tmpl = histByRun[rid]
		}
		row := liveRunRow{
			RunID: rid, Prefix: prefixForRun(rid), Template: tmpl,
			Count: len(ns),
		}
		if len(ns) > 40 {
			row.Namespaces = append([]string(nil), ns[:40]...)
		} else {
			row.Namespaces = ns
		}
		out.LiveRuns = append(out.LiveRuns, row)
	}
	sort.Slice(out.LiveRuns, func(i, j int) bool { return out.LiveRuns[i].RunID > out.LiveRuns[j].RunID })
	return out, nil
}

const managedIndexTTL = 8 * time.Second

func (s *Server) managedMetas(ctx context.Context) ([]nsMeta, string, error) {
	cluster := s.currentCluster().Name
	s.indexMu.Lock()
	if time.Since(s.indexAt) < managedIndexTTL && s.indexCluster == cluster && (s.indexMetas != nil || s.indexErr != nil) {
		metas, err := s.indexMetas, s.indexErr
		s.indexMu.Unlock()
		return metas, cluster, err
	}
	s.indexMu.Unlock()

	cl, err := s.liveClient(20, 40)
	if err != nil {
		return nil, cluster, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	var metas []nsMeta
	if live, ok := cl.(*kube.Live); ok && live != nil {
		infos, err := live.ListManagedNamespaceInfo(ctx, "")
		if err != nil {
			s.indexMu.Lock()
			s.indexAt, s.indexCluster, s.indexMetas, s.indexErr = time.Now(), cluster, nil, err
			s.indexMu.Unlock()
			return nil, cluster, err
		}
		for _, info := range infos {
			metas = append(metas, nsMeta{name: info.Name, runID: info.RunID, config: info.Config})
		}
	} else {
		allNS, err := cl.ListManagedNamespaces(ctx, "")
		if err != nil {
			s.indexMu.Lock()
			s.indexAt, s.indexCluster, s.indexMetas, s.indexErr = time.Now(), cluster, nil, err
			s.indexMu.Unlock()
			return nil, cluster, err
		}
		for _, name := range allNS {
			metas = append(metas, nsMeta{name: name, runID: runIDFromNS(name)})
		}
	}
	s.indexMu.Lock()
	s.indexAt, s.indexCluster, s.indexMetas, s.indexErr = time.Now(), cluster, metas, nil
	s.indexMu.Unlock()
	return metas, cluster, nil
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
			ID:       fmt.Sprintf("session-%d", now.Unix()),
			Status:   "idle",
			Cluster:  s.currentCluster().Name,
			Template: s.activeTemplateName(),
			Started:  &now,
			Warning:  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		}
		m.cur.attachPersist(s)
	} else {
		m.cur.Cluster = s.currentCluster().Name
		if t := s.activeTemplateName(); t != "" {
			m.cur.Template = t
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
	cur := m.cur
	m.mu.Unlock()
	if cur != nil && cur.Status == "running" {
		cur.abortForCleanup()
	}

	s.mu.Lock()
	if s.cleanupBusy {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, fmt.Errorf("cleanup already in progress"))
		return
	}
	s.cleanupBusy = true
	s.mu.Unlock()

	target, terr := s.snapshotTarget()
	if terr != nil {
		s.mu.Lock()
		s.cleanupBusy = false
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, terr)
		return
	}
	cl, err := target.client(20, 40)
	if err != nil {
		s.mu.Lock()
		s.cleanupBusy = false
		s.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	cluster := target.Name
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
			} else if st, err := s.computeCleanupStatus(context.Background(), body.Template); err == nil && st.Template != nil && st.Template.RunID != "" {
				runIDs = []string{st.Template.RunID}
			} else {
				s.mu.Lock()
				s.cleanupBusy = false
				s.mu.Unlock()
				logf("error", "no previous real run to clean")
				writeError(w, http.StatusBadRequest, fmt.Errorf("no previous real run to clean"))
				return
			}
		} else {
			runIDs = []string{last.RunID}
		}
	case "template":
		if body.Template == "" {
			s.mu.Lock()
			s.cleanupBusy = false
			s.mu.Unlock()
			writeError(w, http.StatusBadRequest, fmt.Errorf("template required for scope=template"))
			return
		}
		seen := map[string]bool{}
		for _, e := range s.runsForTemplate(body.Template) {
			if e.RunID != "" && !seen[e.RunID] {
				runIDs = append(runIDs, e.RunID)
				seen[e.RunID] = true
			}
		}
		if st, err := s.computeCleanupStatus(context.Background(), body.Template); err == nil && st.Template != nil && st.Template.RunID != "" {
			if !seen[st.Template.RunID] {
				runIDs = append(runIDs, st.Template.RunID)
			}
		}
		if len(runIDs) == 0 {
			s.mu.Lock()
			s.cleanupBusy = false
			s.mu.Unlock()
			logf("error", fmt.Sprintf("no recorded or live runs for template %q", body.Template))
			writeError(w, http.StatusBadRequest, fmt.Errorf("no recorded or live runs for template %q", body.Template))
			return
		}
	case "all":
		runIDs = []string{""} // empty = all managed
	default:
		s.mu.Lock()
		s.cleanupBusy = false
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, fmt.Errorf("scope must be last, template, or all"))
		return
	}

	logf("info", fmt.Sprintf("start scope=%s template=%s %s dryRun=%v wait=%v (async; independent of HTTP/route timeout)",
		body.Scope, orEmpty(body.Template, "—"), target.logLine(), body.DryRun, body.Wait))
	if body.Scope == "all" {
		logf("warn", "targeting ALL managed namespaces (any run / any template) on this cluster")
	} else {
		for _, rid := range runIDs {
			logf("info", fmt.Sprintf("target run %s (%s)", rid, prefixForRun(rid)))
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true,
		"async":    true,
		"scope":    body.Scope,
		"dryRun":   body.DryRun,
		"cluster":  cluster,
		"target":   target,
		"run":      sink.snapshotSlim(),
		"stream":   eventsPath,
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		"note":     "Cleanup runs in the background; watch the live log over SSE /api/v1/events.",
	})

	go s.runCleanupJob(cl, runIDs, body.Scope, body.Template, body.Wait, body.DryRun, cluster, sink)
}

func (s *Server) isCleanupBusy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupBusy
}

func (s *Server) invalidateManagedIndex() {
	s.indexMu.Lock()
	s.indexAt = time.Time{}
	s.indexMetas = nil
	s.indexErr = nil
	s.indexMu.Unlock()
}

func (s *Server) runCleanupJob(cl kube.Cluster, runIDs []string, scope, template string, wait, dryRun bool, cluster string, sink *execRun) {
	defer func() {
		s.mu.Lock()
		s.cleanupBusy = false
		s.mu.Unlock()
		s.invalidateManagedIndex()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		st, err := s.computeCleanupStatus(ctx, template)
		cancel()
		if err == nil {
			s.publishCleanup(st)
		} else {
			s.eventBus().Publish("cleanup", cluster, template, map[string]any{
				"cleaning": false,
				"cluster":  cluster,
			})
		}
	}()

	s.eventBus().Publish("cleanup", cluster, template, map[string]any{
		"cleaning": true,
		"cluster":  cluster,
		"scope":    scope,
	})

	var logLines []report.CleanupLogLine
	logf := func(level, msg string) {
		sink.appendLog(level, "CLEANUP", 0, msg)
		logLines = append(logLines, report.CleanupLogLine{At: time.Now(), Level: level, Message: msg})
	}

	// Detached from HTTP request — survives OpenShift route ~30s idle timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
	defer cancel()

	var watch *cleanupwatch.Watcher
	if live, ok := cl.(*kube.Live); ok && live.Clientset() != nil {
		watch = cleanupwatch.Start(live.Clientset(), 15*time.Second, func(level, msg string) {
			sink.appendLog(level, "CLUSTER", 0, msg)
			logLines = append(logLines, report.CleanupLogLine{At: time.Now(), Level: level, Message: msg})
		})
	}

	started := time.Now()
	hadErr := false
	var lastErr string
	deleted := 0
	remaining := 0
	var allNS []string
	var totals report.CleanupObjectTotals
	seenNS := map[string]bool{}

	for _, rid := range runIDs {
		nsCount := 0
		if names, err := cl.ListManagedNamespaces(ctx, rid); err == nil {
			nsCount = len(names)
			totals.Namespaces += nsCount
		}
		waitTO := runner.CleanupWaitTimeout(nsCount)
		if nsCount > 0 {
			logf("info", fmt.Sprintf("wait budget %s for ~%d namespace(s) on %s", waitTO, nsCount, cluster))
		}
		res, err := runner.Cleanup(ctx, runner.CleanupOptions{
			Cluster:     cl,
			RunID:       rid,
			DryRun:      dryRun,
			Wait:        wait && !dryRun,
			WaitTimeout: waitTO,
			Log:         func(msg string) { logf("info", msg) },
		})
		if res != nil {
			deleted += len(res.Namespaces)
			remaining += len(res.Remaining)
			for _, n := range res.Namespaces {
				if !seenNS[n] {
					allNS = append(allNS, n)
					seenNS[n] = true
				}
			}
		}
		if err != nil {
			hadErr = true
			lastErr = err.Error()
			logf("error", err.Error())
		}
	}

	fin := time.Now()
	dur := fin.Sub(started)
	status := "passed"
	if hadErr && deleted > 0 {
		status = "partial"
	} else if hadErr {
		status = "failed"
	}
	if hadErr {
		logf("error", fmt.Sprintf("finished with errors in %s · attempted %d NS", dur.Round(time.Millisecond), deleted))
	} else {
		logf("info", fmt.Sprintf("done in %s · %d namespace(s)", dur.Round(time.Millisecond), deleted))
	}

	doc := &report.CleanupReport{
		Scope:      scope,
		Template:   template,
		Cluster:    cluster,
		DryRun:     dryRun,
		Waited:     wait && !dryRun,
		Status:     status,
		RunIDs:     append([]string(nil), runIDs...),
		Started:    started,
		Finished:   fin,
		Targeted:   totals,
		DeletedNS:  deleted,
		Remaining:  remaining,
		Namespaces: allNS,
		Error:      lastErr,
		Logs:       logLines,
	}
	if watch != nil {
		obs := watch.Stop()
		co := &report.ClusterObservation{
			Summary:           obs.Summary,
			MaxNotReady:       obs.MaxNotReady,
			MaxNotReadyDurSec: obs.MaxNotReadyDurSec,
			MonitoringOOM:     obs.MonitoringOOM,
			WorstNodes:        append([]string(nil), obs.WorstNodes...),
		}
		for _, s := range obs.Samples {
			co.Samples = append(co.Samples, report.ClusterSample{
				At: s.At, NodesReady: s.NodesReady, NodesNotReady: s.NodesNotReady,
				MemoryPressure: s.MemoryPressure, DiskPressure: s.DiskPressure, PIDPressure: s.PIDPressure,
				MonitoringReady: s.MonitoringReady, MonitoringTotal: s.MonitoringTotal,
				MonitoringOOM: s.MonitoringOOM, MonitoringRestarts: s.MonitoringRestarts,
				NotReadyNodes: append([]string(nil), s.NotReadyNodes...),
			})
		}
		for _, in := range obs.Incidents {
			co.Incidents = append(co.Incidents, report.ClusterIncident{
				At: in.At, Kind: in.Kind, Message: in.Message, Node: in.Node,
			})
		}
		doc.ClusterObservation = co
		logf("info", "CLUSTER observation: "+obs.Summary)
	}
	if id, err := report.WriteCleanupReport(s.RunDir, doc); err != nil {
		logf("warn", "cleanup report save failed: "+err.Error())
	} else {
		logf("info", "cleanup report saved · "+id+" · open Cleanup reports")
	}
}
