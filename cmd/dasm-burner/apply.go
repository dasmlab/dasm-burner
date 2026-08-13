package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/dasmlab/dasm-burner/internal/burner"
	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func newApplyCmd(gf *globalFlags) *cobra.Command {
	var (
		configPath string
		outDir     string
		dryRun     bool
		confirm    bool
		allowLarge bool
		skipBase   bool
		mode       string
		batchSize  int
		conc       int
		image      string
		waitReady  bool
		timeout    time.Duration
		doMeasure  bool
	)
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a topology to the cluster in sequential, batch, or rate mode",
		Long: `apply creates namespaces, services, deployments, and routes.

WARNING: NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT.

Real applies require --i-understand-this-loads-the-control-plane.
More than 10 namespaces also require --allow-large.

Batch size and API concurrency are independent: a batch of 50 namespaces
can still be written with only 20 concurrent API calls.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrDefault(configPath)
			if err != nil {
				return err
			}
			if mode != "" {
				cfg.Deployment.Mode = mode
			}
			if cmd.Flags().Changed("batch-size") {
				cfg.Deployment.BatchSize = batchSize
			}
			if cmd.Flags().Changed("concurrency") {
				cfg.Deployment.APIConcurrency = conc
			}
			if image != "" {
				cfg.Application.Image = image
			}
			if cmd.Flags().Changed("wait") {
				cfg.Deployment.WaitForReady = waitReady
			}
			if cmd.Flags().Changed("timeout") {
				cfg.Deployment.ReadinessTimeout = config.Duration(timeout)
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := runner.EnsureSafe(cfg, dryRun, confirm, allowLarge); err != nil {
				return err
			}
			g, err := topology.Generate(cfg)
			if err != nil {
				return err
			}

			opts := runner.Options{
				Config:       cfg,
				Graph:        g,
				DryRun:       dryRun,
				SkipBaseline: skipBase,
				Log: func(phase runner.Phase, batch int, msg string) {
					if batch > 0 {
						fmt.Fprintf(os.Stderr, "[%s] batch %d: %s\n", phase, batch, msg)
					} else {
						fmt.Fprintf(os.Stderr, "[%s] %s\n", phase, msg)
					}
				},
			}
			if !dryRun {
				qps := float32(cfg.Deployment.APIConcurrency)
				if qps < 20 {
					qps = 20
				}
				cl, err := kube.NewLive(gf.kubeconfig, qps, int(qps)*2)
				if err != nil {
					return err
				}
				opts.Cluster = cl
			}

			fmt.Fprintf(os.Stderr, "apply run-id=%s seed=%d mode=%s ns=%d dry-run=%v\n",
				g.RunID, g.Seed, cfg.Deployment.Mode, g.Counts.Namespaces, dryRun)

			kbDir := filepath.Join(outDir, "kube-burner")
			collected := filepath.Join(kbDir, "collected")
			var (
				kbFiles     *burner.Files
				prom        *burner.Prometheus
				measureProc *burner.MeasureProc
				kbBin       string
				indexStart  time.Time
			)
			if outDir != "" {
				if err := os.MkdirAll(kbDir, 0o755); err != nil {
					return err
				}
				promURL, tokenFile := "", ""
				if !dryRun && doMeasure {
					p, err := burner.DiscoverPrometheus(context.Background(), gf.kubeconfig, filepath.Join(kbDir, "prometheus.token"))
					if err != nil {
						fmt.Fprintf(os.Stderr, "prometheus discover: %v\n", err)
					} else {
						prom = p
						promURL, tokenFile = p.URL, p.TokenFile
					}
				}
				kbFiles, err = burner.WriteDir(kbDir, cfg, g, promURL, tokenFile, collected)
				if err != nil {
					return err
				}
			}
			if doMeasure && !dryRun {
				kbBin, err = burner.FindBinary()
				if err != nil {
					return err
				}
				if kbFiles == nil {
					return fmt.Errorf("--measure requires --out")
				}
				userMeta, _ := burner.WriteUserMetadata(kbDir, burner.UserMetadata{
					RunID:             g.RunID,
					Prefix:            burner.FormatPrefix(g.RunID),
					DasmBurnerVersion: version,
					DryRun:            false,
					Namespaces:        g.Counts.Namespaces,
					Services:          g.Counts.Services,
					Routes:            g.Counts.Routes,
					Deployments:       g.Counts.Deployments,
					Pods:              g.Counts.Pods,
				})
				dur := 2 * time.Minute
				nBatches := max(1, (g.Counts.Namespaces+max(cfg.Deployment.BatchSize, 1)-1)/max(cfg.Deployment.BatchSize, 1))
				if nBatches > 2 {
					dur = time.Duration(nBatches)*45*time.Second + time.Minute
				}
				indexStart = time.Now()
				measureProc, err = burner.StartMeasure(context.Background(), kbBin, kbFiles.MeasureConfig, gf.kubeconfig, g.RunID, userMeta, dur)
				if err != nil {
					return fmt.Errorf("start kube-burner measure: %w", err)
				}
				fmt.Fprintf(os.Stderr, "kube-burner measure started (pid %d duration %s)\n", measureProc.Cmd.Process.Pid, dur)
			}

			rep, err := runner.Run(context.Background(), opts)
			if measureProc != nil {
				fmt.Fprintf(os.Stderr, "waiting for kube-burner measure to flush pod/service latency\n")
				if werr := measureProc.Wait(); werr != nil {
					fmt.Fprintf(os.Stderr, "measure: %v\n", werr)
				}
				if kbBin != "" && prom != nil && kbFiles != nil {
					end := time.Now()
					if indexStart.IsZero() {
						indexStart = end.Add(-5 * time.Minute)
					}
					metaPath := filepath.Join(kbDir, "user-metadata.yml")
					if ierr := burner.Index(context.Background(), kbBin, gf.kubeconfig, prom.URL, prom.TokenFile, kbFiles.MetricsProfile, collected, g.RunID, metaPath, indexStart.Add(-30*time.Second), end); ierr != nil {
						fmt.Fprintf(os.Stderr, "kube-burner index: %v\n", ierr)
					}
					if aerr := burner.CheckAlerts(context.Background(), kbBin, prom.URL, prom.TokenFile, kbFiles.AlertsProfile, collected, g.RunID, indexStart.Add(-30*time.Second), end); aerr != nil {
						fmt.Fprintf(os.Stderr, "kube-burner check-alerts: %v\n", aerr)
					}
				}
			}
			if outDir != "" {
				_ = topology.WriteRunDir(outDir, configPath, cfg, g)
				if werr := runner.WriteReport(outDir, rep); werr != nil {
					fmt.Fprintf(os.Stderr, "write report: %v\n", werr)
				}
			}
			if rep != nil {
				printApplySummary(rep)
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&configPath, "config", "", "path to OpenShiftNetworkDensity YAML")
	f.StringVar(&outDir, "out", "./run", "write plan.json + apply-report.json here")
	f.BoolVar(&dryRun, "dry-run", false, "plan batches without contacting the cluster")
	f.BoolVar(&confirm, "i-understand-this-loads-the-control-plane", false, "required for a real apply")
	f.BoolVar(&allowLarge, "allow-large", false, "allow more than 10 namespaces")
	f.BoolVar(&skipBase, "skip-baseline", false, "skip the baseline wait")
	f.StringVar(&mode, "mode", "", "override deployment.mode (sequential|batch|rate)")
	f.IntVar(&batchSize, "batch-size", 0, "override deployment.batchSize")
	f.IntVar(&conc, "concurrency", 0, "override deployment.apiConcurrency")
	f.StringVar(&image, "image", "", "override application.image")
	f.BoolVar(&waitReady, "wait", true, "wait for namespace/deployment/route readiness per batch")
	f.DurationVar(&timeout, "timeout", 0, "override deployment.readinessTimeout")
	f.BoolVar(&doMeasure, "measure", false, "run kube-burner measure+index alongside apply")
	return cmd
}

func printApplySummary(rep *runner.Report) {
	fmt.Printf("\nrun-id: %s  mode: %s  dry-run: %v  duration: %s\n",
		rep.RunID, rep.Mode, rep.DryRun, rep.Duration.Round(time.Millisecond))
	if rep.Cluster != "" {
		fmt.Printf("cluster: %s\n", rep.Cluster)
	}
	for _, b := range rep.Batches {
		fmt.Printf("\nBatch %03d  ns=%d svc=%d rt=%d deploy=%d pods=%d  create=%s\n",
			b.ID, b.Namespaces, b.Services, b.Routes, b.Deployments, b.Pods, b.CreateDur.Round(time.Millisecond))
		fmt.Printf("  ns %s\n  svc %s\n  rt %s\n  deploy %s\n", b.NS, b.Svc, b.Rt, b.Dep)
		if b.Ready.Duration > 0 {
			fmt.Printf("  ready ns=%d deploy=%d routes=%d in %s\n",
				b.Ready.NamespacesReady, b.Ready.DeploymentsReady, b.Ready.RoutesAdmitted, b.Ready.Duration.Round(time.Millisecond))
		}
		if len(b.Errors) > 0 {
			fmt.Printf("  errors: %v\n", b.Errors)
		}
	}
	fmt.Printf("\nconvergence: overall %.1f%%  ns %.1f  svc %.1f  rt %.1f  deploy %.1f  ready-pods %.1f\n",
		rep.Convergence.Overall, rep.Convergence.Namespaces, rep.Convergence.Services,
		rep.Convergence.Routes, rep.Convergence.Deployments, rep.Convergence.ReadyPods)
	if len(rep.Errors) > 0 {
		fmt.Printf("errors: %v\n", rep.Errors)
	}
}

func newStatusCmd(gf *globalFlags) *cobra.Command {
	var (
		configPath string
		runID      string
		asJSON     bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show desired vs actual objects for a managed run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runID == "" && configPath == "" {
				return fmt.Errorf("pass --run-id or --config")
			}
			var desired config.Counts
			if configPath != "" {
				cfg, err := loadOrDefault(configPath)
				if err != nil {
					return err
				}
				g, err := topology.Generate(cfg)
				if err != nil {
					return err
				}
				desired = g.Counts
				if runID == "" {
					runID = g.RunID
				}
			}
			cl, err := kube.NewLive(gf.kubeconfig, 20, 40)
			if err != nil {
				return err
			}
			snap, err := cl.ListManaged(context.Background(), runID)
			if err != nil {
				return err
			}
			conv := kube.ComputeConvergence(desired, snap)
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"runId": runID, "convergence": conv, "actual": snap, "desired": desired})
			}
			fmt.Printf("run-id: %s\n", runID)
			fmt.Printf("actual:  ns=%d svc=%d rt=%d deploy=%d pods=%d readyPods=%d\n",
				snap.Namespaces, snap.Services, snap.Routes, snap.Deployments, snap.Pods, snap.ReadyPods)
			if desired.Namespaces > 0 {
				fmt.Printf("desired: ns=%d svc=%d rt=%d deploy=%d pods=%d\n",
					desired.Namespaces, desired.Services, desired.Routes, desired.Deployments, desired.Pods)
				fmt.Printf("convergence: %.1f%%\n", conv.Overall)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "topology YAML (resolves run-id from seed + desired counts)")
	cmd.Flags().StringVar(&runID, "run-id", "", "run id label (kb-{runId}-...)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newCleanupCmd(gf *globalFlags) *cobra.Command {
	var (
		configPath string
		runID      string
		all        bool
		yes        bool
		dryRun     bool
		wait       bool
		timeout    time.Duration
	)
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete namespaces created by dasm-burner (cascades to all objects)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" && runID == "" {
				cfg, err := loadOrDefault(configPath)
				if err != nil {
					return err
				}
				g, err := topology.Generate(cfg)
				if err != nil {
					return err
				}
				runID = g.RunID
			}
			if runID == "" && !all {
				return fmt.Errorf("pass --run-id, --config, or --all")
			}
			if !dryRun && !yes {
				return fmt.Errorf("refusing to delete namespaces without --yes (or pass --dry-run)")
			}
			cl, err := kube.NewLive(gf.kubeconfig, 20, 40)
			if err != nil {
				return err
			}
			res, err := runner.Cleanup(context.Background(), runner.CleanupOptions{
				Cluster:     cl,
				RunID:       runID,
				DryRun:      dryRun,
				Wait:        wait,
				WaitTimeout: timeout,
			})
			if res != nil {
				label := "Would delete"
				if !dryRun {
					label = "Deleted"
				}
				fmt.Printf("%s %d namespace(s) (run-id=%q)\n", label, len(res.Namespaces), res.RunID)
				for _, n := range res.Namespaces {
					fmt.Printf("  - %s\n", n)
				}
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&configPath, "config", "", "topology YAML (resolves run-id from seed)")
	f.StringVar(&runID, "run-id", "", "delete only this run")
	f.BoolVar(&all, "all", false, "delete every dasm-burner managed namespace")
	f.BoolVar(&yes, "yes", false, "actually delete")
	f.BoolVar(&dryRun, "dry-run", false, "list namespaces that would be deleted")
	f.BoolVar(&wait, "wait", false, "wait for namespaces to terminate")
	f.DurationVar(&timeout, "timeout", 10*time.Minute, "wait timeout")
	return cmd
}
