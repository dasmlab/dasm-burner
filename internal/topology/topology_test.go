package topology

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dasmlab/dasm-burner/internal/config"
)

func smokeConfig() *config.Config {
	c := config.Default()
	c.Metadata.Name = "smoke"
	c.Topology.Namespaces.Count = 2
	c.Topology.Services.PerNamespace = 2
	c.Topology.Routes.PerNamespace = 2
	c.Topology.Workloads.ReplicasPerService = 3
	c.Naming.Seed = config.Seed{Value: 1837291}
	return c
}

func TestGenerateCountsAndPairs(t *testing.T) {
	cfg := smokeConfig()
	g, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if g.Seed != 1837291 {
		t.Fatalf("seed %d", g.Seed)
	}
	if len(g.Namespaces) != 2 {
		t.Fatalf("ns=%d", len(g.Namespaces))
	}
	var pairs int
	for _, ns := range g.Namespaces {
		if len(ns.Pairs) != 2 {
			t.Fatalf("ns %s pairs=%d", ns.Name, len(ns.Pairs))
		}
		seen := map[string]bool{}
		for _, p := range ns.Pairs {
			pairs++
			if p.Service == "" || p.Route == "" || p.Deployment == "" || p.App == "" {
				t.Fatalf("empty name in pair %+v", p)
			}
			if p.Replicas != 3 {
				t.Fatalf("replicas %d", p.Replicas)
			}
			if seen[p.App] {
				t.Fatalf("duplicate app %s", p.App)
			}
			seen[p.App] = true
		}
	}
	if pairs != 4 {
		t.Fatalf("pairs=%d", pairs)
	}
	if g.Counts.Pods != 12 {
		t.Fatalf("pods=%d", g.Counts.Pods)
	}
}

func TestSameSeedSameGraph(t *testing.T) {
	a, err := Generate(smokeConfig())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(smokeConfig())
	if err != nil {
		t.Fatal(err)
	}
	if a.Namespaces[0].Name != b.Namespaces[0].Name {
		t.Fatalf("%s vs %s", a.Namespaces[0].Name, b.Namespaces[0].Name)
	}
	if a.Namespaces[0].Pairs[0].Route != b.Namespaces[0].Pairs[0].Route {
		t.Fatal("route names diverged")
	}
}

func TestRenderRoutePointsAtService(t *testing.T) {
	cfg := smokeConfig()
	g, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := Render(g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	routes := string(docs.Routes)
	for _, ns := range g.Namespaces {
		for _, p := range ns.Pairs {
			if !strings.Contains(routes, "name: "+p.Service) {
				t.Fatalf("route YAML missing service %s", p.Service)
			}
			if !strings.Contains(routes, "name: "+p.Route) {
				t.Fatalf("route YAML missing route %s", p.Route)
			}
		}
	}
	if !strings.Contains(string(docs.Deployments), "fieldPath: metadata.name") {
		t.Fatal("deployment missing POD_NAME fieldRef")
	}
	if !strings.Contains(string(docs.Deployments), "/healthz") {
		t.Fatal("deployment missing /healthz probe")
	}
	if strings.Contains(string(docs.Namespaces), "creationTimestamp") {
		t.Fatal("namespaces YAML still has creationTimestamp")
	}
	if strings.Contains(string(docs.Deployments), "\nstatus:") {
		t.Fatal("deployments YAML still has status")
	}
}

func TestWriteRunDir(t *testing.T) {
	cfg := smokeConfig()
	g, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := WriteRunDir(dir, "", cfg, g); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"rendered-config.yaml",
		"plan.json",
		"objects/namespaces.yaml",
		"objects/services.yaml",
		"objects/routes.yaml",
		"objects/deployments.yaml",
	} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty %s", name)
		}
	}
}
