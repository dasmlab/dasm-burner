package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/dasmlab/dasm-burner/internal/auth"
	"github.com/dasmlab/dasm-burner/internal/ui"
)

func newServeCmd(gf *globalFlags) *cobra.Command {
	var (
		addr       string
		runDir     string
		configPath string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the HAWT UI and cluster-health API (does not apply load)",
		Long: `serve is observational: plan, status, health, topology canvas, and the last report.
It does not create namespaces. Apply remains a CLI action with safety flags.
When KEYCLOAK_URL + OIDC_CLIENT_SECRET are set, the UI requires the dasm-burner admin role.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			staticRoot, err := fs.Sub(staticEmbed, "static")
			if err != nil {
				return err
			}
			authSvc, err := auth.New(context.Background(), auth.ConfigFromEnv())
			if err != nil {
				return fmt.Errorf("oidc: %w", err)
			}
			if authSvc.Enabled() {
				fmt.Fprintf(os.Stderr, "OIDC enabled (client dasm-burner)\n")
			} else {
				fmt.Fprintf(os.Stderr, "OIDC disabled (set KEYCLOAK_URL + OIDC_CLIENT_SECRET to enable)\n")
			}
			srv := ui.New(version, runDir, configPath, gf.kubeconfig, staticRoot, authSvc)
			hs := &http.Server{
				Addr:              addr,
				Handler:           srv.Mux,
				ReadHeaderTimeout: 10 * time.Second,
			}
			fmt.Fprintf(os.Stderr, "dasm-burner UI %s on %s (run-dir %s)\n", version, addr, runDir)
			return hs.ListenAndServe()
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "listen address")
	cmd.Flags().StringVar(&runDir, "run-dir", "./run", "directory with apply-report.json / collected metrics")
	cmd.Flags().StringVar(&configPath, "config", "", "topology YAML for plan/status (default built-in counts)")
	return cmd
}
