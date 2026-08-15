package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

type Options struct {
	Cluster      kube.Cluster
	Config       *config.Config
	Graph        *topology.Graph
	DryRun       bool
	SkipBaseline bool
	Log          func(phase Phase, batch int, msg string)
	PollInterval time.Duration
}

func Run(ctx context.Context, opts Options) (*Report, error) {
	if opts.Config == nil || opts.Graph == nil {
		return nil, fmt.Errorf("config and graph are required")
	}
	cfg := opts.Config
	g := opts.Graph
	rep := &Report{
		RunID:   g.RunID,
		Seed:    g.Seed,
		Mode:    cfg.Deployment.Mode,
		DryRun:  opts.DryRun,
		Started: time.Now(),
	}
	log := func(phase Phase, batch int, msg string) {
		rep.event(phase, batch, msg)
		if opts.Log != nil {
			opts.Log(phase, batch, msg)
		}
	}

	log(PhasePrecheck, 0, "starting precheck")
	if err := precheck(ctx, opts, rep); err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		log(PhaseAborted, 0, err.Error())
		rep.Finished = time.Now()
		rep.Duration = rep.Finished.Sub(rep.Started)
		return rep, err
	}

	if !opts.SkipBaseline {
		d := cfg.Monitoring.Baseline.Duration.Std()
		if d > 0 && !opts.DryRun {
			log(PhaseBaseline, 0, fmt.Sprintf("waiting %s (Prometheus collection is Phase 3)", d))
			if err := sleep(ctx, d); err != nil {
				return finish(rep, err)
			}
		} else {
			log(PhaseBaseline, 0, "skipped")
		}
	} else {
		log(PhaseBaseline, 0, "skipped")
	}

	plan := PlanBatches(cfg, g.Namespaces)
	ApplyBatchPlan(cfg, plan)
	batches := planBatches(cfg, g.Namespaces)
	log(PhasePrecheck, 0, fmt.Sprintf("mode=%s strategy=%s batches=%d namespaces=%d size=%d concurrency=%d",
		plan.Mode, plan.Strategy, len(batches), len(g.Namespaces), plan.Size, cfg.Deployment.APIConcurrency))
	log(PhasePrecheck, 0, plan.Reason)

	gate := &abortGate{}
	switch cfg.Deployment.Mode {
	case config.DeployRate:
		if err := runRate(ctx, opts, rep, log, gate); err != nil {
			return finish(rep, err)
		}
	default:
		for i, batch := range batches {
			if err := runBatch(ctx, opts, rep, log, i+1, batch, gate); err != nil {
				return finish(rep, err)
			}
			if i < len(batches)-1 {
				delay := cfg.Deployment.BatchDelay.Std()
				if n := len(rep.Batches); n > 0 {
					last := rep.Batches[n-1]
					if last.HealthLevel == "WARNING" || last.Health.MastersMemoryPressure > 0 || last.Health.MastersNotReady > 0 {
						extra := 45 * time.Second
						if delay < extra {
							delay = extra
						} else {
							delay *= 2
						}
						log(PhaseHealthCheck, i+1, fmt.Sprintf("CONTROLPLANE settle %s before next wave", delay.Round(time.Second)))
					}
				}
				if err := sleep(ctx, delay); err != nil {
					return finish(rep, err)
				}
			}
		}
	}

	log(PhaseFinalSettle, 0, "settle")
	if !opts.DryRun {
		_ = sleep(ctx, cfg.Deployment.BatchDelay.Std())
	}
	log(PhaseFinalMeasurement, 0, "convergence")
	if !opts.DryRun && opts.Cluster != nil {
		snap, err := opts.Cluster.ListManaged(ctx, g.RunID)
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
		} else {
			rep.Convergence = kube.ComputeConvergence(g.Counts, snap)
		}
		if h, err := opts.Cluster.ClusterHealth(ctx, g.RunID); err == nil {
			rep.Health = h
		}
	} else {
		rep.Convergence = kube.ComputeConvergence(g.Counts, kube.Snapshot{
			Namespaces:  g.Counts.Namespaces,
			Services:    g.Counts.Services,
			Routes:      g.Counts.Routes,
			Deployments: g.Counts.Deployments,
			Pods:        g.Counts.Pods,
			ReadyPods:   g.Counts.Pods,
		})
	}
	log(PhaseReport, 0, fmt.Sprintf("overall convergence %.1f%%", rep.Convergence.Overall))
	log(PhaseDone, 0, "complete")
	return finish(rep, nil)
}

func planBatches(cfg *config.Config, namespaces []topology.Namespace) [][]topology.Namespace {
	batches, _ := SplitByPlan(cfg, namespaces)
	return batches
}

func runBatch(ctx context.Context, opts Options, rep *Report, log func(Phase, int, string), id int, batch []topology.Namespace, gate *abortGate) error {
	br := BatchReport{
		ID:          id,
		Namespaces:  len(batch),
		Services:    pairCount(batch),
		Routes:      pairCount(batch),
		Deployments: pairCount(batch),
		Pods:        podCount(batch),
	}
	log(PhaseBatchStart, id, fmt.Sprintf("namespaces=%d objects=%d", len(batch), br.Services+br.Routes+br.Deployments))

	start := time.Now()
	log(PhaseObjectCreation, id, "creating")
	if err := createBatch(ctx, opts, &br, batch); err != nil && !opts.DryRun {
		br.CreateDur = time.Since(start)
		br.Errors = append(br.Errors, err.Error())
		rep.Batches = append(rep.Batches, br)
		return err
	}
	br.CreateDur = time.Since(start)

	if opts.Config.Deployment.WaitForReady && !opts.DryRun {
		log(PhaseWaitForReady, id, "waiting")
		timeout := opts.Config.Deployment.ReadinessTimeout.Std()
		st, err := kube.WaitReady(ctx, opts.Cluster, batch, timeout, opts.PollInterval)
		br.Ready = st
		if err != nil {
			br.Errors = append(br.Errors, err.Error())
			log(PhaseWaitForReady, id, err.Error())
			rep.Batches = append(rep.Batches, br)
			rep.Errors = append(rep.Errors, err.Error())
			return err
		}
	} else {
		log(PhaseWaitForReady, id, "skipped")
	}

	log(PhaseBatchMeasurement, id, "snapshot")
	if !opts.DryRun && opts.Cluster != nil {
		snap, err := opts.Cluster.ListManaged(ctx, opts.Graph.RunID)
		if err == nil {
			br.Convergence = kube.ComputeConvergence(opts.Graph.Counts, snap)
		}
	}
	if err := checkHealth(ctx, opts, rep, &br, log, id, gate); err != nil {
		rep.Batches = append(rep.Batches, br)
		return err
	}
	rep.Batches = append(rep.Batches, br)
	return nil
}

func runRate(ctx context.Context, opts Options, rep *Report, log func(Phase, int, string), gate *abortGate) error {
	rate := opts.Config.Deployment.NamespacesPerSec
	if rate <= 0 {
		rate = 1
	}
	interval := time.Duration(float64(time.Second) / rate)
	var batch []topology.Namespace
	id := 0
	for i, ns := range opts.Graph.Namespaces {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		tick := time.Now()
		id++
		batch = []topology.Namespace{ns}
		if err := runBatch(ctx, opts, rep, log, id, batch, gate); err != nil {
			return err
		}
		if i < len(opts.Graph.Namespaces)-1 {
			elapsed := time.Since(tick)
			if wait := interval - elapsed; wait > 0 {
				if err := sleep(ctx, wait); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func createBatch(ctx context.Context, opts Options, br *BatchReport, batch []topology.Namespace) error {
	if opts.DryRun || opts.Cluster == nil {
		br.NS.Created = len(batch)
		n := pairCount(batch)
		br.Svc.Created = n
		br.Rt.Created = n
		br.Dep.Created = n
		return nil
	}

	conc := opts.Config.Deployment.APIConcurrency
	if conc < 1 {
		conc = 1
	}

	var mu sync.Mutex
	track := func(err error, c *CreateCounts) error {
		mu.Lock()
		defer mu.Unlock()
		return classifyCreate(err, c)
	}

	if err := runLimited(ctx, conc, len(batch), func(i int) error {
		return track(opts.Cluster.CreateNamespace(ctx, topology.BuildNamespace(opts.Graph, batch[i])), &br.NS)
	}); err != nil {
		return err
	}

	if secret := opts.Config.Application.ImagePullSecret; secret != "" {
		from := opts.Config.Application.ImagePullSecretFrom
		if from == "" {
			return fmt.Errorf("application.imagePullSecret is set but imagePullSecretFrom is empty")
		}
		for _, ns := range batch {
			if err := opts.Cluster.CopySecret(ctx, from, secret, ns.Name); err != nil {
				return fmt.Errorf("copy pull secret into %s: %w", ns.Name, err)
			}
		}
	}

	type item struct {
		ns   topology.Namespace
		pair topology.Pair
	}
	var items []item
	for _, ns := range batch {
		for _, p := range ns.Pairs {
			items = append(items, item{ns, p})
		}
	}

	if err := runLimited(ctx, conc, len(items), func(i int) error {
		svc := topology.BuildService(opts.Graph, items[i].ns, items[i].pair, opts.Config)
		return track(opts.Cluster.CreateService(ctx, svc), &br.Svc)
	}); err != nil {
		return err
	}
	if err := runLimited(ctx, conc, len(items), func(i int) error {
		d := topology.BuildDeployment(opts.Graph, items[i].ns, items[i].pair, opts.Config)
		return track(opts.Cluster.CreateDeployment(ctx, d), &br.Dep)
	}); err != nil {
		return err
	}
	if err := runLimited(ctx, conc, len(items), func(i int) error {
		rt := topology.BuildRoute(opts.Graph, items[i].ns, items[i].pair, opts.Config)
		return track(opts.Cluster.CreateRoute(ctx, rt), &br.Rt)
	}); err != nil {
		return err
	}
	if br.NS.Failed+br.Svc.Failed+br.Rt.Failed+br.Dep.Failed > 0 {
		return fmt.Errorf("create failures ns=%d svc=%d rt=%d deploy=%d", br.NS.Failed, br.Svc.Failed, br.Rt.Failed, br.Dep.Failed)
	}
	return nil
}

func classifyCreate(err error, c *CreateCounts) error {
	switch {
	case err == nil:
		c.Created++
		return nil
	case apierrors.IsAlreadyExists(err):
		c.Existing++
		return nil
	default:
		c.Failed++
		return err
	}
}

func runLimited(ctx context.Context, conc, n int, fn func(i int) error) error {
	if n == 0 {
		return nil
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(i); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return first
}

func precheck(ctx context.Context, opts Options, rep *Report) error {
	if opts.DryRun {
		rep.Cluster = "dry-run"
		return nil
	}
	if opts.Cluster == nil {
		return fmt.Errorf("cluster client is required for a real apply")
	}
	ver, err := opts.Cluster.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("precheck server version: %w", err)
	}
	rep.Cluster = ver
	ok, err := opts.Cluster.HasRouteAPI(ctx)
	if err != nil {
		return fmt.Errorf("precheck route API: %w", err)
	}
	if !ok {
		return fmt.Errorf("precheck: OpenShift Route API (route.openshift.io/v1) is not available")
	}
	return nil
}

func pairCount(batch []topology.Namespace) int {
	n := 0
	for _, ns := range batch {
		n += len(ns.Pairs)
	}
	return n
}

func podCount(batch []topology.Namespace) int {
	n := 0
	for _, ns := range batch {
		for _, p := range ns.Pairs {
			n += p.Replicas
		}
	}
	return n
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func finish(rep *Report, err error) (*Report, error) {
	rep.Finished = time.Now()
	rep.Duration = rep.Finished.Sub(rep.Started)
	if err != nil && len(rep.Errors) == 0 {
		rep.Errors = append(rep.Errors, err.Error())
	}
	return rep, err
}

type abortGate struct {
	since time.Time
}

func checkHealth(ctx context.Context, opts Options, rep *Report, br *BatchReport, log func(Phase, int, string), id int, gate *abortGate) error {
	if opts.DryRun || opts.Cluster == nil || !opts.Config.Safety.Enabled {
		log(PhaseHealthCheck, id, "skipped")
		return nil
	}
	h, err := opts.Cluster.ClusterHealth(ctx, opts.Graph.RunID)
	if err != nil {
		log(PhaseHealthCheck, id, "health probe failed: "+err.Error())
		return nil
	}
	rep.Health = h
	br.Health = h
	d := Evaluate(opts.Config.Safety, h)
	br.HealthLevel = d.Level
	if !d.Abort {
		gate.since = time.Time{}
		log(PhaseHealthCheck, id, fmt.Sprintf("NORMAL nodes=%d/%d · %s oom=%d",
			h.NodesReady, h.NodesReady+h.NodesNotReady, d.Pulse, h.OOMKilled))
		return nil
	}
	grace := opts.Config.Safety.GracePeriod.Std()
	// Control-plane / etcd failures: shorter grace — do not keep creating into a dying etcd.
	if h.MastersNotReady > 0 || (h.EtcdPods > 0 && h.EtcdReady < h.EtcdPods) {
		if grace > 15*time.Second {
			grace = 15 * time.Second
		}
	}
	if grace > 0 {
		if gate.since.IsZero() {
			gate.since = time.Now()
			log(PhaseHealthCheck, id, "WARNING "+d.Reason+" · "+d.Pulse+" (grace "+grace.String()+")")
			br.HealthLevel = "WARNING"
			return nil
		}
		if time.Since(gate.since) < grace {
			log(PhaseHealthCheck, id, "WARNING "+d.Reason+" · "+d.Pulse+" (still in grace)")
			br.HealthLevel = "WARNING"
			return nil
		}
	}
	rep.Aborted = true
	rep.AbortReason = d.Reason
	log(PhaseAborted, id, d.Reason+" · "+d.Pulse)
	return fmt.Errorf("abort: %s", d.Reason)
}

func WriteReport(dir string, rep *Report) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "apply-report.json"), b, 0o644)
}
