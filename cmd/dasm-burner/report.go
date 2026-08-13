package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/report"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func newReportCmd(gf *globalFlags) *cobra.Command {
	var (
		configPath string
		outDir     string
		runID      string
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Freeze an immutable OVN/node + apply narrative for a run",
		Long: `report merges <out>/apply-report.json and <out>/kube-burner/collected into an
immutable archive under <out>/reports/<snapshotId>/.

When apply-report.json already contains end-of-run health, that is used for the
Close box (cleanup-safe). Otherwise a live ClusterHealth sample fills the gap.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrDefault(configPath)
			if err != nil {
				return err
			}
			g, err := topology.Generate(cfg)
			if err != nil {
				return err
			}
			if runID == "" {
				runID = g.RunID
			}
			var apply *runner.Report
			if b, err := os.ReadFile(filepath.Join(outDir, "apply-report.json")); err == nil {
				_ = json.Unmarshal(b, &apply)
			}

			fin := time.Now()
			started := fin
			status := "snapshot"
			if apply != nil {
				if !apply.Started.IsZero() {
					started = apply.Started
				}
				if !apply.Finished.IsZero() {
					fin = apply.Finished
				}
				status = "passed"
				if apply.Aborted {
					status = "aborted"
				}
			}

			needLive := apply == nil || apply.Health.SampledAt.IsZero()
			var live kube.Health
			if needLive {
				cl, err := kube.NewLive(gf.kubeconfig, 20, 40)
				if err != nil {
					return err
				}
				live, err = cl.ClusterHealth(context.Background(), runID)
				if err != nil {
					return fmt.Errorf("cluster health: %w", err)
				}
				if apply == nil {
					apply = &runner.Report{RunID: runID, Started: started, Finished: fin, Health: live}
				} else {
					apply.Health = live
				}
			}

			meta := report.Meta{
				Prefix:   "kb-" + runID,
				Status:   status,
				Started:  started,
				Finished: fin,
				Desired: report.DesiredCounts{
					Namespaces:  g.Counts.Namespaces,
					Services:    g.Counts.Services,
					Routes:      g.Counts.Routes,
					Deployments: g.Counts.Deployments,
					Pods:        g.Counts.Pods,
				},
			}
			frozen, err := report.Freeze(apply, filepath.Join(outDir, "kube-burner", "collected"), meta)
			if err != nil {
				return err
			}
			if frozen.RunID == "" {
				frozen.RunID = runID
			}
			id, err := report.WriteSnapshot(outDir, frozen, apply)
			if err != nil {
				return err
			}
			fmt.Print(frozen.Narrative)
			fmt.Fprintf(os.Stderr, "wrote immutable snapshot %s under %s/reports/\n", id, outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "topology YAML (resolves run-id from seed)")
	cmd.Flags().StringVar(&outDir, "out", "./run", "directory with apply-report.json and kube-burner/collected")
	cmd.Flags().StringVar(&runID, "run-id", "", "override run id")
	return cmd
}
