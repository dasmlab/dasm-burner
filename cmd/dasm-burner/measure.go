package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/dasmlab/dasm-burner/internal/burner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func newRenderKubeBurnerCmd() *cobra.Command {
	var (
		configPath string
		outDir     string
	)
	cmd := &cobra.Command{
		Use:   "render-kube-burner",
		Short: "Write kube-burner measure/init configs from a topology (does not run kube-burner)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrDefault(configPath)
			if err != nil {
				return err
			}
			g, err := topology.Generate(cfg)
			if err != nil {
				return err
			}
			dir := filepath.Join(outDir, "kube-burner")
			files, err := burner.WriteDir(dir, cfg, g, "", "", filepath.Join(dir, "collected"))
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Wrote %s (run-id=%s kube-burner %s)\n", files.Dir, g.RunID, burner.KubeBurnerVersion)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to OpenShiftNetworkDensity YAML")
	cmd.Flags().StringVar(&outDir, "out", "./run", "output directory")
	return cmd
}

func newMeasureCmd(gf *globalFlags) *cobra.Command {
	var (
		configPath string
		outDir     string
		duration   time.Duration
		runID      string
	)
	cmd := &cobra.Command{
		Use:   "measure",
		Short: "Run kube-burner measure + Prometheus index against an existing run",
		Long: `measure watches pods/services (podLatency, serviceLatency) and then
indexes OpenShift Prometheus (thanos-querier) into <out>/kube-burner/collected.

Requires the kube-burner binary (v2.8.1) on PATH or ./bin/kube-burner.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrDefault(configPath)
			if err != nil {
				return err
			}
			g, err := topology.Generate(cfg)
			if err != nil {
				return err
			}
			if runID != "" {
				g.RunID = runID
			}
			bin, err := burner.FindBinary()
			if err != nil {
				return err
			}
			dir := filepath.Join(outDir, "kube-burner")
			tokenFile := filepath.Join(dir, "prometheus.token")
			collected := filepath.Join(dir, "collected")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			prom, err := burner.DiscoverPrometheus(context.Background(), gf.kubeconfig, tokenFile)
			if err != nil {
				return err
			}
			files, err := burner.WriteDir(dir, cfg, g, prom.URL, prom.TokenFile, collected)
			if err != nil {
				return err
			}
			start := time.Now()
			fmt.Fprintf(os.Stderr, "kube-burner measure run-id=%s duration=%s selector=%s\n",
				g.RunID, duration, topology.Selector(g.RunID))
			proc, err := burner.StartMeasure(context.Background(), bin, files.MeasureConfig, gf.kubeconfig, g.RunID, duration)
			if err != nil {
				return err
			}
			if err := proc.Cmd.Wait(); err != nil {
				fmt.Fprintf(os.Stderr, "measure: %v\n", err)
			}
			end := time.Now()
			if err := burner.Index(context.Background(), bin, gf.kubeconfig, prom.URL, prom.TokenFile, files.MetricsProfile, collected, g.RunID, start.Add(-30*time.Second), end); err != nil {
				fmt.Fprintf(os.Stderr, "index: %v\n", err)
			}
			if err := burner.CheckAlerts(context.Background(), bin, prom.URL, prom.TokenFile, files.AlertsProfile, collected, g.RunID, start.Add(-30*time.Second), end); err != nil {
				fmt.Fprintf(os.Stderr, "check-alerts: %v\n", err)
			}
			fmt.Fprintf(os.Stderr, "metrics in %s\n", collected)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "topology YAML (resolves run-id from seed)")
	cmd.Flags().StringVar(&outDir, "out", "./run", "output directory")
	cmd.Flags().StringVar(&runID, "run-id", "", "override run id selector")
	cmd.Flags().DurationVar(&duration, "duration", 10*time.Minute, "how long kube-burner measure watches")
	return cmd
}
