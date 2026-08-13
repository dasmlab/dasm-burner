package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
		Short: "Write an OVN/node/events + kube-burner narrative for a run",
		Long: `report snapshots cluster health (nodes, OVN pods, OOM, events),
merges <out>/apply-report.json and <out>/kube-burner/collected, and writes
report.json + report.md.`,
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
			cl, err := kube.NewLive(gf.kubeconfig, 20, 40)
			if err != nil {
				return err
			}
			h, err := cl.ClusterHealth(context.Background(), runID)
			if err != nil {
				return fmt.Errorf("cluster health: %w", err)
			}
			var apply *runner.Report
			if b, err := os.ReadFile(filepath.Join(outDir, "apply-report.json")); err == nil {
				_ = json.Unmarshal(b, &apply)
			}
			doc, err := report.Build(h, apply, filepath.Join(outDir, "kube-burner", "collected"))
			if err != nil {
				return err
			}
			if doc.RunID == "" {
				doc.RunID = runID
			}
			if err := report.Write(outDir, doc); err != nil {
				return err
			}
			fmt.Print(doc.Narrative)
			fmt.Fprintf(os.Stderr, "wrote %s/report.json and report.md\n", outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "topology YAML (resolves run-id from seed)")
	cmd.Flags().StringVar(&outDir, "out", "./run", "directory with apply-report.json and kube-burner/collected")
	cmd.Flags().StringVar(&runID, "run-id", "", "override run id")
	return cmd
}
