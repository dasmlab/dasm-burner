package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartingTemplateValidates(t *testing.T) {
	c := StartingTemplate()
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	got := c.Counts()
	if got.Namespaces != 2 || got.Services != 4 || got.Pods != 12 {
		t.Fatalf("starting counts = %+v", got)
	}
}

func TestDefaultValidates(t *testing.T) {
	c := Default()
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	got := c.Counts()
	if got.Namespaces != 2500 || got.Services != 5000 || got.Routes != 5000 || got.Deployments != 5000 || got.Pods != 15000 {
		t.Fatalf("default counts = %+v", got)
	}
	if got.Intended != 2500+5000+5000+5000 {
		t.Fatalf("intended = %d", got.Intended)
	}
}

func TestLoadSparseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	body := `
apiVersion: benchmark.dasmlab.org/v1
kind: OpenShiftNetworkDensity
metadata:
  name: smoke
topology:
  namespaces:
    count: 2
naming:
  seed: 42
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Topology.Services.PerNamespace != 2 {
		t.Fatalf("services defaulted to %d", c.Topology.Services.PerNamespace)
	}
	if c.Naming.Seed.Value != 42 || c.Naming.Seed.Auto {
		t.Fatalf("seed = %+v", c.Naming.Seed)
	}
	if c.Counts().Pods != 12 {
		t.Fatalf("smoke pods = %d want 12", c.Counts().Pods)
	}
}

func TestOneToOneMismatch(t *testing.T) {
	c := Default()
	c.Topology.Routes.PerNamespace = 3
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "oneToOne") {
		t.Fatalf("expected oneToOne error, got %v", err)
	}
}

func TestPhase1RejectsLaterControllers(t *testing.T) {
	c := Default()
	c.Topology.Workloads.Controller = ControllerStatefulSet
	if err := Validate(c); err == nil {
		t.Fatal("expected StatefulSet to be rejected in Phase 1")
	}
}
