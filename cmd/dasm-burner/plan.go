package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func newPlanCmd() *cobra.Command {
	var (
		configPath string
		asJSON     bool
	)
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Print intended object counts for a topology config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrDefault(configPath)
			if err != nil {
				return err
			}
			g, err := topology.Generate(cfg)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"runId":  g.RunID,
					"seed":   g.Seed,
					"counts": g.Counts,
				})
			}
			c := g.Counts
			fmt.Printf("run:    %s\n", cfg.Metadata.Name)
			fmt.Printf("run-id: %s\n", g.RunID)
			fmt.Printf("seed:   %d\n", g.Seed)
			fmt.Printf("\n")
			fmt.Printf("namespaces:  %d\n", c.Namespaces)
			fmt.Printf("services:    %d\n", c.Services)
			fmt.Printf("routes:      %d\n", c.Routes)
			fmt.Printf("deployments: %d\n", c.Deployments)
			fmt.Printf("pods:        %d  (via Deployment replicas)\n", c.Pods)
			fmt.Printf("pairs:       %d  (route↔service 1:1)\n", c.Pairs)
			fmt.Printf("intended:    %d  (ns+svc+route+deploy; controllers add more)\n", c.Intended)
			fmt.Printf("\nUse apply --dry-run to see batches; apply requires --i-understand-this-loads-the-control-plane.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to OpenShiftNetworkDensity YAML (defaults if omitted)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newGenerateCmd() *cobra.Command {
	var (
		configPath string
		outDir     string
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Write named Namespace/Service/Route/Deployment YAML (no cluster apply)",
		Long: `generate builds the topology graph, resolves naming.seed, and writes:

  <out>/config.yaml
  <out>/rendered-config.yaml
  <out>/plan.json
  <out>/objects/{namespaces,services,routes,deployments}.yaml

Nothing is applied to a cluster. Use apply for that.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadOrDefault(configPath)
			if err != nil {
				return err
			}
			g, err := topology.Generate(cfg)
			if err != nil {
				return err
			}
			if err := topology.WriteRunDir(outDir, configPath, cfg, g); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Wrote %s (run-id=%s seed=%d intended=%d)\n",
				outDir, g.RunID, g.Seed, g.Counts.Intended)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to OpenShiftNetworkDensity YAML (defaults if omitted)")
	cmd.Flags().StringVar(&outDir, "out", "./run", "output directory")
	return cmd
}

func loadOrDefault(path string) (*config.Config, error) {
	if path == "" {
		c := config.Default()
		if err := config.Validate(c); err != nil {
			return nil, err
		}
		return c, nil
	}
	return config.Load(path)
}
