package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dasmlab/dasm-burner/internal/burner"
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
	LogSeq      uint64             `json:"logSeq,omitempty"`
	Stream      string             `json:"stream,omitempty"`
	mu          sync.Mutex         `json:"-"`
	cancel      context.CancelFunc `json:"-"`
	onChange    func()             `json:"-"`
	publishLog  func(line logLine) `json:"-"`
}

type execManager struct {
	mu  sync.Mutex
	cur *execRun
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
				"stream":  eventsPath,
				"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"run":     cur.snapshotSlim(),
			"stream":  eventsPath,
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
		Template          string   `json:"template"`
		DryRun            bool     `json:"dryRun"`
		Confirm           bool     `json:"confirm"`
		AllowLarge        bool     `json:"allowLarge"`
		AllowOverCapacity bool     `json:"allowOverCapacity"`
		SkipBase          bool     `json:"skipBaseline"`
		EnableOVNDiag     *bool    `json:"enableOVNDiag"`
		AvoidTaints       []string `json:"avoidTaints"`
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
	// Execute-time override: list of kubectl-style taints pods must NOT tolerate.
	// Omit / null → keep template/defaults. Explicit [] → clear (allow all).
	if body.AvoidTaints != nil {
		parsed, perr := config.ParseAvoidTaints(body.AvoidTaints)
		if perr != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("avoidTaints: %w", perr))
			return
		}
		cfg.Application.AvoidTaints = parsed
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
	batchPlan := runner.PlanBatches(cfg, g.Namespaces)
	runner.ApplyBatchPlan(cfg, batchPlan)

	target, terr := s.snapshotTarget()
	if terr != nil {
		writeError(w, http.StatusBadRequest, terr)
		return
	}

	var capacity kube.DensityCapacity
	if !body.DryRun {
		qps := float32(cfg.Deployment.APIConcurrency)
		if qps < 20 {
			qps = 20
		}
		cl, cerr := target.client(qps, int(qps)*2)
		if cerr != nil {
			writeError(w, http.StatusServiceUnavailable, cerr)
			return
		}
		live, ok := cl.(*kube.Live)
		if !ok || live.Clientset() == nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("capacity precheck requires a live cluster client"))
			return
		}
		podsPerNS := 0
		if g.Counts.Namespaces > 0 {
			podsPerNS = g.Counts.Pods / g.Counts.Namespaces
		}
		capCtx, capCancel := context.WithTimeout(r.Context(), 20*time.Second)
		capacity, cerr = kube.EvaluateDensityCapacity(capCtx, live.Clientset(), cfg.Application.AvoidTaints, g.Counts.Pods, batchPlan.Size, podsPerNS)
		capCancel()
		if cerr != nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Errorf("capacity precheck: %w", cerr))
			return
		}
		if err := kube.CheckDensityFit(capacity, body.AllowOverCapacity); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
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
		Cluster:     target.Name,
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
	run.attachPersist(s)
	m.cur = run
	m.mu.Unlock()

	if !body.DryRun {
		s.recordHistory(runHistoryEntry{
			RunID:    g.RunID,
			Template: body.Template,
			Prefix:   pfx,
			Seed:     g.Seed,
			DryRun:   false,
			Started:  now,
			Status:   "running",
			Cluster:  run.Cluster,
		})
	}
	s.writeCurrentExec(run)

	run.appendLog("info", "PREFIX", 0, fmt.Sprintf("common prefix %s · pattern %s-ns-00001-xxxx", pfx, pfx))
	run.appendLog("info", "TARGET", 0, target.logLine())
	if capacity.Summary != "" {
		lvl := "info"
		if !capacity.FitsRun || !capacity.FitsWave {
			lvl = "warn"
		}
		run.appendLog(lvl, "CAPACITY", 0, capacity.Summary)
		if body.AllowOverCapacity && (!capacity.FitsRun || !capacity.FitsWave) {
			run.appendLog("warn", "CAPACITY", 0, "proceeding despite over-capacity (allowOverCapacity=true)")
		}
	}
	run.appendLog("info", "BATCH", 0, fmt.Sprintf("%s · size=%d waves=%d · ~%d objs/wave (units/NS=%d target≤%d) · %s",
		batchPlan.Strategy, batchPlan.Size, batchPlan.Count,
		batchPlan.ObjectsPerWave, batchPlan.ObjectsPerNS, batchPlan.TargetWaveObjs, batchPlan.Reason))
	if len(cfg.Application.AvoidTaints) == 0 {
		run.appendLog("info", "SCHED", 0, "avoidTaints=none (pods may land on any node the scheduler allows)")
	} else {
		parts := make([]string, 0, len(cfg.Application.AvoidTaints))
		for _, t := range cfg.Application.AvoidTaints {
			parts = append(parts, t.String())
		}
		run.appendLog("info", "SCHED", 0, "will NOT tolerate taints: "+strings.Join(parts, ", ")+
			" · nodeAffinity excludes matching infra/role labels")
	}

	enableOVN := true
	if body.EnableOVNDiag != nil {
		enableOVN = *body.EnableOVNDiag
	}
	if enableOVN {
		run.appendLog("info", "OVNDIAG", 0, "diagnoser samples ENABLED for this run (baseline + watch + per-batch)")
	} else {
		run.appendLog("info", "OVNDIAG", 0, "diagnoser samples DISABLED for this run")
	}

	go s.executeRun(ctx, run, cfg, g, body.DryRun, body.SkipBase, enableOVN, target)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"run":           run.snapshot(),
		"avoidTaints":   cfg.Application.AvoidTaints,
		"cluster":       target,
		"capacity":      capacity,
		"enableOVNDiag": enableOVN,
		"warning":       "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
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

// plannedBatches uses the same smart PlanBatches / SplitByPlan as the runner.
func plannedBatches(cfg *config.Config, namespaces []topology.Namespace) [][]topology.Namespace {
	batches, _ := runner.SplitByPlan(cfg, namespaces)
	return batches
}

func SplitBatchCount(cfg *config.Config, ns int) int {
	fake := make([]topology.Namespace, ns)
	for i := range fake {
		fake[i].Index = i + 1
	}
	plan := runner.PlanBatches(cfg, fake)
	return plan.Count
}

func (s *Server) executeRun(ctx context.Context, run *execRun, cfg *config.Config, g *topology.Graph, dryRun, skipBase, enableOVN bool, target clusterTarget) {
	var (
		lastRep     *runner.Report
		openHealth  kube.Health
		measureProc *burner.MeasureProc
		kbBin       string
		kbFiles     *burner.Files
		prom        *burner.Prometheus
		indexStart  time.Time
		userMeta    string
	)
	collected := filepath.Join(s.RunDir, "kube-burner", "collected")
	kbDir := filepath.Join(s.RunDir, "kube-burner")

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
		s.writeCurrentExec(run)
	}()

	run.setStep("precheck", stepRunning, fmt.Sprintf("%s · starting", run.Prefix))
	run.appendLog("info", "PRECHECK", 0, fmt.Sprintf("template=%s run=%s prefix=%s dryRun=%v %s", run.Template, g.RunID, run.Prefix, dryRun, target.logLine()))

	var cl kube.Cluster
	if !dryRun {
		var err error
		qps := float32(cfg.Deployment.APIConcurrency)
		if qps < 20 {
			qps = 20
		}
		cl, err = target.client(qps, int(qps)*2)
		if err != nil {
			run.fail("precheck", err.Error())
			return
		}
	}
	run.setStep("precheck", stepPassed, fmt.Sprintf("%s · ok", run.Prefix))

	if cl != nil {
		if h, err := cl.ClusterHealth(ctx, g.RunID); err == nil {
			openHealth = h
			run.appendLog("info", "OPEN", 0, fmt.Sprintf("health snapshot nodes Ready %d/%d OVN %d/%d restarts=%d",
				h.NodesReady, h.NodesReady+h.NodesNotReady, h.OVNReady, h.OVNPods, h.OVNRestarts))
		}
	if live, ok := cl.(*kube.Live); ok && live.Clientset() != nil && enableOVN {
		run.appendLog("info", "OVNDIAG", 0, "baseline sample queued on OVN worker (not on the web request path)")
		s.performOVNJob(ovnJob{kind: "baseline", log: run})
		run.appendLog("info", "OVNDIAG", 0, "per-batch samples go through the same single-slot worker; no 45s watch loop")
	}
	}

	if !dryRun && !cfg.IsObjectPressure() {
		_ = os.MkdirAll(kbDir, 0o755)
		promURL, tokenFile := "", ""
		if p, err := burner.DiscoverPrometheus(ctx, target.Kubeconfig, filepath.Join(kbDir, "prometheus.token")); err != nil {
			run.appendLog("warn", "MEASURE", 0, "prometheus discover: "+err.Error()+" (index/alerts skipped)")
		} else {
			prom = p
			promURL, tokenFile = p.URL, p.TokenFile
			run.appendLog("info", "MEASURE", 0, "prometheus "+p.URL)
		}
		var werr error
		kbFiles, werr = burner.WriteDir(kbDir, cfg, g, promURL, tokenFile, collected)
		if werr != nil {
			run.appendLog("warn", "MEASURE", 0, "render kube-burner: "+werr.Error())
		} else {
			userMeta, _ = burner.WriteUserMetadata(kbDir, burner.UserMetadata{
				RunID:             g.RunID,
				Prefix:            run.Prefix,
				Cluster:           run.Cluster,
				Template:          run.Template,
				DasmBurnerVersion: s.Version,
				DryRun:            false,
				Namespaces:        g.Counts.Namespaces,
				Services:          g.Counts.Services,
				Routes:            g.Counts.Routes,
				Deployments:       g.Counts.Deployments,
				Pods:              g.Counts.Pods,
			})
		}
		if kbFiles != nil {
			if bin, err := burner.FindBinary(); err != nil {
				run.appendLog("warn", "MEASURE", 0, err.Error())
			} else {
				kbBin = bin
				dur := 2 * time.Minute
				nBatches := SplitBatchCount(cfg, g.Counts.Namespaces)
				if nBatches > 2 {
					dur = time.Duration(nBatches)*45*time.Second + time.Minute
				}
				indexStart = time.Now()
				mp, err := burner.StartMeasure(ctx, kbBin, kbFiles.MeasureConfig, target.Kubeconfig, g.RunID, userMeta, dur)
				if err != nil {
					run.appendLog("warn", "MEASURE", 0, "start measure: "+err.Error())
				} else {
					measureProc = mp
					run.appendLog("info", "MEASURE", 0, fmt.Sprintf("kube-burner measure started duration=%s", dur))
				}
			}
		}
	}

	if cfg.IsObjectPressure() {
		s.executeObjectPressure(ctx, run, cfg, g, dryRun, target, kbDir, collected)
		return
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
			if phase == runner.PhaseBatchMeasurement || phase == runner.PhaseFinalMeasurement || phase == runner.PhaseBatchStart {
				run.logMem(batch, string(phase))
			}
			// OVN diagnoser: sample after each batch measurement + final (not kube-burner metrics).
			if enableOVN && !dryRun && (phase == runner.PhaseBatchMeasurement || phase == runner.PhaseFinalMeasurement) {
				bid := batch
				if !s.enqueueOVN("sample", bid, run) {
					run.appendLog("warn", "OVNDIAG", bid, "OVN worker busy — sample skipped")
				}
			}
		},
	}

	rep, err := runner.Run(ctx, opts)
	lastRep = rep

	if measureProc != nil {
		run.appendLog("info", "MEASURE", 0, "waiting for kube-burner measure to flush")
		if werr := measureProc.Wait(); werr != nil {
			run.appendLog("warn", "MEASURE", 0, werr.Error())
		}
		end := time.Now()
		if indexStart.IsZero() {
			indexStart = end.Add(-5 * time.Minute)
		}
		if kbBin != "" && prom != nil && kbFiles != nil {
			run.appendLog("info", "INDEX", 0, "kube-burner index → "+collected)
			if ierr := burner.Index(context.Background(), kbBin, target.Kubeconfig, prom.URL, prom.TokenFile, kbFiles.MetricsProfile, collected, g.RunID, userMeta, indexStart.Add(-30*time.Second), end); ierr != nil {
				run.appendLog("warn", "INDEX", 0, ierr.Error())
			} else {
				run.appendLog("info", "INDEX", 0, "metrics indexed (local + tarball)")
			}
			run.appendLog("info", "ALERTS", 0, "kube-burner check-alerts")
			if aerr := burner.CheckAlerts(context.Background(), kbBin, prom.URL, prom.TokenFile, kbFiles.AlertsProfile, collected, g.RunID, indexStart.Add(-30*time.Second), end); aerr != nil {
				run.appendLog("warn", "ALERTS", 0, aerr.Error())
			}
		}
	}

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

func (s *Server) executeObjectPressure(ctx context.Context, run *execRun, cfg *config.Config, g *topology.Graph, dryRun bool, target clusterTarget, kbDir, collected string) {
	run.setStep("baseline", stepPassed, "object-pressure · kube-burner init")
	run.appendLog("info", "APPLY", 0, "ObjectPressure basetype — kube-burner init (not client-go density apply)")
	run.logMem(0, "object-pressure-start")
	if dryRun {
		run.appendLog("info", "APPLY", 0, "dry-run: would render init.yml and run kube-burner init")
		run.mu.Lock()
		run.Status = "passed"
		run.mu.Unlock()
		run.setStep("done", stepPassed, "dry-run complete")
		return
	}
	cfg = filterPressureObjects(ctx, run, cfg, target)
	if cfg == nil {
		return
	}
	_ = os.MkdirAll(kbDir, 0o755)
	files, err := burner.WriteDir(kbDir, cfg, g, "", "", collected)
	if err != nil {
		run.fail("baseline", "render object-pressure: "+err.Error())
		return
	}
	bin, err := burner.FindBinary()
	if err != nil {
		run.fail("baseline", err.Error())
		return
	}
	run.appendLog("info", "APPLY", 0, "kube-burner init -c "+files.InitConfig)
	run.setStep("batch-1", stepRunning, "kube-burner init")
	if err := burner.RunInit(ctx, bin, files.InitConfig, target.Kubeconfig, g.RunID); err != nil {
		run.fail("batch-1", "kube-burner init: "+err.Error())
		return
	}
	run.setStep("batch-1", stepPassed, "init complete")
	run.logMem(0, "object-pressure-done")
	if snap, err := func() (kube.Snapshot, error) {
		cl, err := target.client(20, 40)
		if err != nil {
			return kube.Snapshot{}, err
		}
		return cl.ListManaged(ctx, g.RunID)
	}(); err == nil {
		run.appendLog("info", "STATE", 0, fmt.Sprintf("live after init · NS=%d routes=%d svcs=%d pods=%d/%d",
			snap.Namespaces, snap.Routes, snap.Services, snap.ReadyPods, snap.Pods))
	}
	run.mu.Lock()
	run.Status = "passed"
	run.mu.Unlock()
	run.setStep("done", stepPassed, "complete · open Report")
}

// filterPressureObjects drops unavailable optional GVKs (warn) and fails if a required GVK is missing.
func filterPressureObjects(ctx context.Context, run *execRun, cfg *config.Config, target clusterTarget) *config.Config {
	cl, err := target.client(20, 40)
	if err != nil {
		run.fail("baseline", "object-pressure client: "+err.Error())
		return nil
	}
	live, ok := cl.(*kube.Live)
	if !ok || live == nil {
		run.appendLog("warn", "APPLY", 0, "object-pressure: no Live discovery client — skipping GVK availability checks")
		return cfg
	}
	cp := *cfg
	cp.Topology.Objects = append([]config.PressureObject(nil), cfg.Topology.Objects...)
	var kept []config.PressureObject
	for _, o := range cp.Topology.Objects {
		if !o.Enabled {
			kept = append(kept, o)
			continue
		}
		ok, err := live.HasAPIResource(ctx, o.APIVersion, o.Kind)
		if err != nil {
			run.appendLog("warn", "APPLY", 0, fmt.Sprintf("GVK check %s/%s: %v", o.APIVersion, o.Kind, err))
			if o.Required {
				run.fail("baseline", fmt.Sprintf("required object %s (%s/%s): discovery failed: %v", o.ID, o.APIVersion, o.Kind, err))
				return nil
			}
			run.appendLog("warn", "APPLY", 0, fmt.Sprintf("skipping %s — discovery error", o.ID))
			o.Enabled = false
			kept = append(kept, o)
			continue
		}
		if !ok {
			msg := fmt.Sprintf("API %s kind %s not served — skipping %s", o.APIVersion, o.Kind, o.ID)
			if o.Required {
				run.fail("baseline", "required object missing on cluster: "+msg)
				return nil
			}
			run.appendLog("warn", "APPLY", 0, msg)
			o.Enabled = false
			kept = append(kept, o)
			continue
		}
		kept = append(kept, o)
	}
	cp.Topology.Objects = kept
	enabled := 0
	for _, o := range kept {
		if o.Enabled {
			enabled++
		}
	}
	if enabled < 1 {
		run.fail("baseline", "object-pressure: no enabled objects remain after GVK checks")
		return nil
	}
	return &cp
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
	return r.snapshotTail(80)
}

func (r *execRun) snapshotSlim() *execRun {
	return r.snapshotTail(80)
}

func (r *execRun) snapshotMeta() *execRun {
	return r.snapshotTail(0)
}

func (r *execRun) snapshotTail(maxLogs int) *execRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *r
	cp.Steps = append([]runStep(nil), r.Steps...)
	cp.Stream = eventsPath
	if maxLogs <= 0 || len(r.Logs) == 0 {
		cp.Logs = nil
		return &cp
	}
	if len(r.Logs) > maxLogs {
		cp.Logs = append([]logLine(nil), r.Logs[len(r.Logs)-maxLogs:]...)
	} else {
		cp.Logs = append([]logLine(nil), r.Logs...)
	}
	return &cp
}

func (r *execRun) appendLog(level, phase string, batch int, msg string) {
	line := logLine{At: time.Now(), Level: level, Phase: phase, Batch: batch, Message: msg}
	r.mu.Lock()
	r.Logs = append(r.Logs, line)
	const maxLogs = 400
	if len(r.Logs) > maxLogs {
		r.Logs = r.Logs[len(r.Logs)-maxLogs:]
	}
	pub := r.publishLog
	cb := r.onChange
	r.mu.Unlock()
	if pub != nil {
		pub(line)
	}
	if cb != nil {
		cb()
	}
}

func (r *execRun) logMem(batch int, where string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	r.appendLog("info", "MEM", batch, fmt.Sprintf("%s · alloc=%.1fMi heapInuse=%.1fMi sys=%.1fMi goroutines=%d",
		where,
		float64(ms.Alloc)/(1024*1024),
		float64(ms.HeapInuse)/(1024*1024),
		float64(ms.Sys)/(1024*1024),
		runtime.NumGoroutine()))
}

func (r *execRun) setStep(id string, st stepStatus, msg string) {
	r.mu.Lock()
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
		break
	}
	cb := r.onChange
	r.mu.Unlock()
	if cb != nil {
		cb()
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
