package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

type stepStatus string

const (
	stepPending stepStatus = "pending"
	stepRunning stepStatus = "running"
	stepPassed  stepStatus = "passed"
	stepFailed  stepStatus = "failed"
	stepSkipped stepStatus = "skipped"
)

type runStep struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Kind     string     `json:"kind"` // phase | batch
	Batch    int        `json:"batch,omitempty"`
	SeqFrom  int        `json:"seqFrom,omitempty"`
	SeqTo    int        `json:"seqTo,omitempty"`
	Range    string     `json:"range,omitempty"` // e.g. 00001 or 00001--000050
	Status   stepStatus `json:"status"`
	Message  string     `json:"message,omitempty"`
	Started  *time.Time `json:"started,omitempty"`
	Finished *time.Time `json:"finished,omitempty"`
}

type logLine struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Phase   string    `json:"phase,omitempty"`
	Batch   int       `json:"batch,omitempty"`
	Message string    `json:"message"`
}

type execRun struct {
	ID          string             `json:"id"`
	Template    string             `json:"template"`
	Cluster     string             `json:"cluster"`
	DryRun      bool               `json:"dryRun"`
	Status      string             `json:"status"` // idle|running|passed|failed|aborted
	RunID       string             `json:"runId,omitempty"`
	Prefix      string             `json:"prefix,omitempty"`      // kb-{runId}
	NamePattern string             `json:"namePattern,omitempty"` // kb-{runId}-{kind}-{seq}-{sfx}
	Seed        int64              `json:"seed,omitempty"`
	Steps       []runStep          `json:"steps"`
	Logs        []logLine          `json:"logs"`
	Started     *time.Time         `json:"started,omitempty"`
	Finished    *time.Time         `json:"finished,omitempty"`
	Error       string             `json:"error,omitempty"`
	Convergence any                `json:"convergence,omitempty"`
	ReportURL   string             `json:"reportUrl,omitempty"`
	SnapshotID  string             `json:"snapshotId,omitempty"`
	Warning     string             `json:"warning"`
	mu          sync.Mutex         `json:"-"`
	cancel      context.CancelFunc `json:"-"`
}

type execManager struct {
	mu  sync.Mutex
	cur *execRun
}

func (s *Server) execMgr() *execManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exec == nil {
		s.exec = &execManager{}
	}
	return s.exec
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m := s.execMgr()
		m.mu.Lock()
		cur := m.cur
		m.mu.Unlock()
		if cur == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"run":     nil,
				"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"run":     cur.snapshot(),
			"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		})
	case http.MethodPost:
		s.startRun(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) runAction(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/v1/runs/")
	action = strings.Trim(action, "/")
	if action == "current" && r.Method == http.MethodGet {
		s.runs(w, r)
		return
	}
	if action == "cancel" && r.Method == http.MethodPost {
		m := s.execMgr()
		m.mu.Lock()
		cur := m.cur
		m.mu.Unlock()
		if cur != nil && cur.cancel != nil {
			cur.cancel()
			cur.appendLog("warn", "CANCEL", 0, "cancel requested")
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "canceling"})
		return
	}
	if action == "clear" && r.Method == http.MethodPost {
		m := s.execMgr()
		m.mu.Lock()
		cur := m.cur
		m.mu.Unlock()
		if cur == nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
			return
		}
		if cur.Status == "running" {
			writeError(w, http.StatusConflict, fmt.Errorf("cannot clear canvas while a run is in progress"))
			return
		}
		cur.mu.Lock()
		cur.Logs = nil
		cur.mu.Unlock()
		cur.appendLog("info", "CLEAR", 0, "live log canvas cleared")
		writeJSON(w, http.StatusOK, map[string]any{
			"run":     cur.snapshot(),
			"status":  "cleared",
			"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		})
		return
	}
	http.NotFound(w, r)
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Template   string `json:"template"`
		DryRun     bool   `json:"dryRun"`
		Confirm    bool   `json:"confirm"`
		AllowLarge bool   `json:"allowLarge"`
		SkipBase   bool   `json:"skipBaseline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Template == "" {
		body.Template = s.activeTemplateName()
	}
	if body.Template == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("select a saved template first"))
		return
	}
	cfg, err := s.loadTemplate(body.Template)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("template %q: %w", body.Template, err))
		return
	}
	s.setActiveTemplate(body.Template)
	if err := runner.EnsureSafe(cfg, body.DryRun, body.Confirm, body.AllowLarge); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	g, err := topology.Generate(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	m := s.execMgr()
	m.mu.Lock()
	if m.cur != nil && m.cur.Status == "running" {
		m.mu.Unlock()
		writeError(w, http.StatusConflict, fmt.Errorf("a run is already in progress"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	pfx := prefixForRun(g.RunID)
	run := &execRun{
		ID:          fmt.Sprintf("%s-%d", g.RunID, now.Unix()),
		Template:    body.Template,
		Cluster:     s.currentCluster().Name,
		DryRun:      body.DryRun,
		Status:      "running",
		RunID:       g.RunID,
		Prefix:      pfx,
		NamePattern: fmt.Sprintf("%s-{kind}-{seq:05d}-{sfx}", pfx),
		Seed:        g.Seed,
		Started:     &now,
		Warning:     "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		cancel:      cancel,
		Steps:       planSteps(cfg, g),
	}
	m.cur = run
	m.mu.Unlock()

	run.appendLog("info", "PREFIX", 0, fmt.Sprintf("common prefix %s · pattern %s-ns-00001-xxxx", pfx, pfx))

	go s.executeRun(ctx, run, cfg, g, body.DryRun, body.SkipBase)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"run":     run.snapshot(),
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func planSteps(cfg *config.Config, g *topology.Graph) []runStep {
	pfx := prefixForRun(g.RunID)
	steps := []runStep{
		{ID: "precheck", Label: "Precheck", Kind: "phase", Status: stepPending, Message: pfx},
		{ID: "baseline", Label: "Baseline", Kind: "phase", Status: stepPending},
	}
	for i, batch := range plannedBatches(cfg, g.Namespaces) {
		id := i + 1
		from, to := batch[0].Index, batch[len(batch)-1].Index
		rng := formatSeqRange(from, to)
		steps = append(steps, runStep{
			ID:      fmt.Sprintf("batch-%d", id),
			Label:   fmt.Sprintf("Batch %d · ns %s", id, rng),
			Kind:    "batch",
			Batch:   id,
			SeqFrom: from,
			SeqTo:   to,
			Range:   rng,
			Status:  stepPending,
			Message: fmt.Sprintf("%s-ns-%s-*", pfx, rng),
		})
	}
	steps = append(steps,
		runStep{ID: "settle", Label: "Final settle", Kind: "phase", Status: stepPending},
		runStep{ID: "measure", Label: "Convergence", Kind: "phase", Status: stepPending},
		runStep{ID: "report", Label: "Report", Kind: "phase", Status: stepPending},
		runStep{ID: "done", Label: "Done", Kind: "phase", Status: stepPending},
	)
	return steps
}

func formatSeqRange(from, to int) string {
	if to <= from {
		return fmt.Sprintf("%05d", from)
	}
	return fmt.Sprintf("%05d--%05d", from, to)
}

// plannedBatches mirrors runner batching (including rate = 1 NS per batch).
func plannedBatches(cfg *config.Config, namespaces []topology.Namespace) [][]topology.Namespace {
	if cfg.Deployment.Mode == config.DeployRate {
		out := make([][]topology.Namespace, 0, len(namespaces))
		for _, ns := range namespaces {
			out = append(out, []topology.Namespace{ns})
		}
		return out
	}
	size := cfg.Deployment.BatchSize
	if cfg.Deployment.Mode == config.DeploySequential {
		size = 1
	}
	return runner.SplitBatches(namespaces, size)
}

func SplitBatchCount(cfg *config.Config, ns int) int {
	if ns < 1 {
		return 0
	}
	size := cfg.Deployment.BatchSize
	if cfg.Deployment.Mode == config.DeploySequential {
		size = 1
	}
	if size < 1 {
		size = ns
	}
	n := (ns + size - 1) / size
	if cfg.Deployment.Mode == config.DeployRate {
		return ns
	}
	return n
}

func (s *Server) executeRun(ctx context.Context, run *execRun, cfg *config.Config, g *topology.Graph, dryRun, skipBase bool) {
	var (
		lastRep    *runner.Report
		openHealth kube.Health
	)

	defer func() {
		fin := time.Now()
		run.mu.Lock()
		run.Finished = &fin
		status := run.Status
		if status == "running" {
			run.Status = "passed"
			status = "passed"
		}
		pfx := run.Prefix
		tmpl := run.Template
		rid := run.RunID
		seed := run.Seed
		cluster := run.Cluster
		var started time.Time
		if run.Started != nil {
			started = *run.Started
		} else {
			started = fin
		}
		run.mu.Unlock()

		snapID := ""
		if lastRep != nil {
			if id, err := s.persistRunSnapshot(run, g, lastRep, openHealth, status, dryRun); err == nil && id != "" {
				snapID = id
				run.mu.Lock()
				run.SnapshotID = id
				run.ReportURL = "/report?id=" + id
				run.mu.Unlock()
				run.appendLog("info", "REPORT", 0, "immutable snapshot "+id)
			} else if err != nil {
				run.appendLog("warn", "REPORT", 0, "snapshot failed: "+err.Error())
			}
		}

		if !dryRun && rid != "" {
			s.recordHistory(runHistoryEntry{
				RunID:      rid,
				Template:   tmpl,
				Prefix:     pfx,
				Seed:       seed,
				DryRun:     false,
				Started:    started,
				Finished:   fin,
				Status:     status,
				Cluster:    cluster,
				SnapshotID: snapID,
			})
		}
	}()

	run.setStep("precheck", stepRunning, fmt.Sprintf("%s · starting", run.Prefix))
	run.appendLog("info", "PRECHECK", 0, fmt.Sprintf("template=%s run=%s prefix=%s dryRun=%v cluster=%s", run.Template, g.RunID, run.Prefix, dryRun, run.Cluster))

	cs := s.clusterState()
	cs.mu.Lock()
	kc := cs.kubeconfig
	ctxName := cs.context
	cs.mu.Unlock()
	if kc == "" {
		kc = s.Kubeconfig
	}

	var cl kube.Cluster
	if !dryRun {
		var err error
		qps := float32(cfg.Deployment.APIConcurrency)
		if qps < 20 {
			qps = 20
		}
		cl, err = kube.NewLiveContext(kc, ctxName, qps, int(qps)*2)
		if err != nil {
			run.fail("precheck", err.Error())
			return
		}
	}
	run.setStep("precheck", stepPassed, fmt.Sprintf("%s · ok", run.Prefix))

	if cl != nil {
		if h, err := cl.ClusterHealth(ctx, g.RunID); err == nil {
			openHealth = h
			run.appendLog("info", "OPEN", 0, fmt.Sprintf("health snapshot nodes Ready %d/%d OVN %d/%d",
				h.NodesReady, h.NodesReady+h.NodesNotReady, h.OVNReady, h.OVNPods))
		}
	}

	opts := runner.Options{
		Cluster:      cl,
		Config:       cfg,
		Graph:        g,
		DryRun:       dryRun,
		SkipBaseline: skipBase,
		Log: func(phase runner.Phase, batch int, msg string) {
			if phase == runner.PhaseBatchStart {
				run.mu.Lock()
				for _, st := range run.Steps {
					if st.Batch == batch && st.Range != "" {
						msg = fmt.Sprintf("%s · %s-ns-%s-* · %s", st.Range, run.Prefix, st.Range, msg)
						break
					}
				}
				run.mu.Unlock()
			}
			if phase == runner.PhaseWaitForReady && msg == "skipped" {
				msg = "ready wait skipped"
			}
			run.appendLog("info", string(phase), batch, msg)
			run.mapPhase(phase, batch, msg)
		},
	}

	rep, err := runner.Run(ctx, opts)
	lastRep = rep
	if err != nil {
		run.fail("", err.Error())
		if rep != nil {
			run.mu.Lock()
			run.Convergence = rep.Convergence
			run.mu.Unlock()
			_ = writeApplyReport(s.RunDir, rep)
		}
		return
	}
	run.mu.Lock()
	run.Convergence = rep.Convergence
	run.Status = "passed"
	run.mu.Unlock()
	doneMsg := "complete"
	if !dryRun {
		doneMsg = "complete · open Report"
	}
	run.setStep("done", stepPassed, doneMsg)
	_ = writeApplyReport(s.RunDir, rep)
}

func writeApplyReport(runDir string, rep *runner.Report) error {
	if rep == nil {
		return nil
	}
	_ = os.MkdirAll(runDir, 0o755)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "apply-report.json"), b, 0o644)
}

func (r *execRun) snapshot() *execRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *r
	cp.Steps = append([]runStep(nil), r.Steps...)
	cp.Logs = append([]logLine(nil), r.Logs...)
	return &cp
}

func (r *execRun) appendLog(level, phase string, batch int, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Logs = append(r.Logs, logLine{At: time.Now(), Level: level, Phase: phase, Batch: batch, Message: msg})
	if len(r.Logs) > 2000 {
		r.Logs = r.Logs[len(r.Logs)-2000:]
	}
}

func (r *execRun) setStep(id string, st stepStatus, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for i := range r.Steps {
		if r.Steps[i].ID != id {
			continue
		}
		r.Steps[i].Status = st
		r.Steps[i].Message = msg
		if st == stepRunning && r.Steps[i].Started == nil {
			r.Steps[i].Started = &now
		}
		if st == stepPassed || st == stepFailed || st == stepSkipped {
			r.Steps[i].Finished = &now
		}
		return
	}
}

func (r *execRun) fail(stepID, msg string) {
	r.mu.Lock()
	r.Status = "failed"
	r.Error = msg
	r.mu.Unlock()
	if stepID != "" {
		r.setStep(stepID, stepFailed, msg)
	} else {
		// mark current running step failed
		r.mu.Lock()
		for i := range r.Steps {
			if r.Steps[i].Status == stepRunning {
				now := time.Now()
				r.Steps[i].Status = stepFailed
				r.Steps[i].Message = msg
				r.Steps[i].Finished = &now
			}
		}
		r.mu.Unlock()
	}
	r.appendLog("error", "FAILED", 0, msg)
}

func (r *execRun) mapPhase(phase runner.Phase, batch int, msg string) {
	switch phase {
	case runner.PhasePrecheck:
		r.setStep("precheck", stepRunning, msg)
	case runner.PhaseBaseline:
		r.setStep("precheck", stepPassed, "ok")
		r.setStep("baseline", stepRunning, msg)
		if msg == "skipped" {
			r.setStep("baseline", stepSkipped, msg)
		}
	case runner.PhaseBatchStart:
		r.passIfRunning("baseline")
		r.passIfRunning("precheck")
		r.setStep(fmt.Sprintf("batch-%d", batch), stepRunning, msg)
	case runner.PhaseObjectCreation, runner.PhaseWaitForReady, runner.PhaseBatchMeasurement, runner.PhaseHealthCheck:
		r.setStep(fmt.Sprintf("batch-%d", batch), stepRunning, msg)
	case runner.PhaseFinalSettle:
		// close open batches with a stable range/prefix summary (not the last "skipped" wait msg)
		r.mu.Lock()
		pfx := r.Prefix
		for i := range r.Steps {
			if r.Steps[i].Kind == "batch" && (r.Steps[i].Status == stepRunning || r.Steps[i].Status == stepPending) {
				now := time.Now()
				if r.Steps[i].Status == stepPending {
					r.Steps[i].Status = stepSkipped
				} else {
					r.Steps[i].Status = stepPassed
					if r.Steps[i].Range != "" {
						r.Steps[i].Message = fmt.Sprintf("%s · %s-ns-%s-*", r.Steps[i].Range, pfx, r.Steps[i].Range)
					}
				}
				r.Steps[i].Finished = &now
			}
			if r.Steps[i].ID == "baseline" && r.Steps[i].Status == stepRunning {
				now := time.Now()
				r.Steps[i].Status = stepPassed
				r.Steps[i].Finished = &now
			}
		}
		r.mu.Unlock()
		r.setStep("settle", stepRunning, msg)
	case runner.PhaseFinalMeasurement:
		r.setStep("settle", stepPassed, "ok")
		r.setStep("measure", stepRunning, msg)
	case runner.PhaseReport:
		r.setStep("measure", stepPassed, "ok")
		r.setStep("report", stepRunning, msg)
		r.mu.Lock()
		if !r.DryRun {
			r.ReportURL = "/report"
		}
		r.mu.Unlock()
	case runner.PhaseDone:
		r.setStep("report", stepPassed, func() string {
			if r.DryRun {
				return "ok (dry-run)"
			}
			return "ok · report ready"
		}())
		r.setStep("done", stepPassed, msg)
	case runner.PhaseAborted:
		r.fail("", msg)
	}
}

func (r *execRun) passIfRunning(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for i := range r.Steps {
		if r.Steps[i].ID == id && (r.Steps[i].Status == stepRunning || r.Steps[i].Status == stepPending) {
			if r.Steps[i].Status == stepPending && id != "baseline" {
				continue
			}
			if r.Steps[i].Status == stepPending {
				r.Steps[i].Status = stepSkipped
			} else {
				r.Steps[i].Status = stepPassed
			}
			r.Steps[i].Finished = &now
		}
	}
}
