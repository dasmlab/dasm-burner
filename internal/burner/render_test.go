package burner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func TestWriteDir(t *testing.T) {
	cfg := config.Default()
	cfg.Metadata.Name = "smoke"
	cfg.Topology.Namespaces.Count = 2
	cfg.Naming.Seed = config.Seed{Value: 1837291}
	g, err := topology.Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files, err := WriteDir(dir, cfg, g, "https://thanos.example", filepath.Join(dir, "token"), filepath.Join(dir, "collected"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{files.MeasureConfig, files.InitConfig, files.MetricsProfile, files.AlertsProfile, files.MetricsEndpoint} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty %s", p)
		}
	}
	m, _ := os.ReadFile(files.MeasureConfig)
	if !strings.Contains(string(m), "podLatency") || !strings.Contains(string(m), "serviceLatency") || !strings.Contains(string(m), "svcTimeout") {
		t.Fatalf("measure.yml missing measurements:\n%s", m)
	}
	initb, _ := os.ReadFile(files.InitConfig)
	if !strings.Contains(string(initb), "jobIterations: 2") {
		t.Fatalf("init.yml:\n%s", initb)
	}
	dep, _ := os.ReadFile(filepath.Join(files.ObjectTemplates, "deployment.yml"))
	if !strings.Contains(string(dep), "{{.Iteration}}") {
		t.Fatal("deployment template missing kube-burner vars")
	}
}

func TestFindBinaryLooksInBin(t *testing.T) {
	// May or may not be present in CI; just ensure the error is useful.
	if _, err := FindBinary(); err != nil && !strings.Contains(err.Error(), "kube-burner") {
		t.Fatal(err)
	}
}
