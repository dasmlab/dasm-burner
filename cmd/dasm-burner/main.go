package main

import (
	"embed"
	"os"

	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

//go:embed all:static
var staticEmbed embed.FS

type globalFlags struct {
	kubeconfig string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	gf := &globalFlags{}
	root := &cobra.Command{
		Use:   "dasm-burner",
		Short: "OpenShift network-density control plane around kube-burner",
		Long: `dasm-burner generates a controlled OpenShift topology
(namespaces × routes × services × deployments) so you can measure
API / OVN / node pressure.

WARNING: NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT.

  plan       print intended object counts
  generate   write named YAML objects (does not apply)
  apply      create objects in sequential / batch / rate mode
  measure    kube-burner pod/service latency + Prometheus index
  render-kube-burner  write kube-burner YAML from the topology
  status     desired vs actual convergence
  report     OVN/node/events + kube-burner narrative
  serve      HAWT UI (observational; does not apply load)
  cleanup    delete managed namespaces`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&gf.kubeconfig, "kubeconfig", "", "kubeconfig path (else KUBECONFIG or ~/.kube/config)")
	root.AddCommand(newPlanCmd())
	root.AddCommand(newGenerateCmd())
	root.AddCommand(newApplyCmd(gf))
	root.AddCommand(newMeasureCmd(gf))
	root.AddCommand(newRenderKubeBurnerCmd())
	root.AddCommand(newStatusCmd(gf))
	root.AddCommand(newReportCmd(gf))
	root.AddCommand(newServeCmd(gf))
	root.AddCommand(newCleanupCmd(gf))
	return root
}
