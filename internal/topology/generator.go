package topology

import (
	"fmt"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/naming"
)

const (
	LabelManaged = "dasm-burner.dasmlab.org/managed"
	LabelRun     = "dasm-burner.dasmlab.org/run"
	LabelPair    = "dasm-burner.dasmlab.org/pair"
	LabelKind    = "dasm-burner.dasmlab.org/kind"
	LabelConfig  = "dasm-burner.dasmlab.org/config"
	LabelApp     = "app"
)

func Selector(runID string) string {
	s := LabelManaged + "=true"
	if runID != "" {
		s += "," + LabelRun + "=" + runID
	}
	return s
}

// Pair is one route ↔ service ↔ deployment unit inside a namespace.
type Pair struct {
	Index      int    `json:"index" yaml:"index"`
	App        string `json:"app" yaml:"app"`
	Service    string `json:"service" yaml:"service"`
	Route      string `json:"route" yaml:"route"`
	Deployment string `json:"deployment" yaml:"deployment"`
	Replicas   int    `json:"replicas" yaml:"replicas"`
}

// Namespace is the generated per-namespace graph.
type Namespace struct {
	Name  string `json:"name" yaml:"name"`
	Index int    `json:"index" yaml:"index"`
	Pairs []Pair `json:"pairs" yaml:"pairs"`
}

// Graph is the fully named topology. It is the Phase 1 source of truth;
// renderers (YAML, later kube-burner templates) project from it.
type Graph struct {
	RunID      string        `json:"runId" yaml:"runId"`
	Seed       int64         `json:"seed" yaml:"seed"`
	ConfigName string        `json:"configName" yaml:"configName"`
	Namespaces []Namespace   `json:"namespaces" yaml:"namespaces"`
	Counts     config.Counts `json:"counts" yaml:"counts"`
}

func Generate(cfg *config.Config) (*Graph, error) {
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	factory := naming.NewFactory(cfg.Naming)
	// Persist the resolved seed so generate writes a reproducible config.
	cfg.Naming.Seed = config.Seed{Auto: false, Value: factory.Seed()}

	g := &Graph{
		RunID:      factory.RunID(),
		Seed:       factory.Seed(),
		ConfigName: cfg.Metadata.Name,
		Counts:     cfg.Counts(),
	}

	nsCount := cfg.Topology.Namespaces.Count
	pairsPerNS := cfg.Topology.Services.PerNamespace
	replicas := cfg.Topology.Workloads.ReplicasPerService
	// ObjectPressure creates namespaces via kube-burner init; graph only needs NS count + RunID.
	if cfg.IsObjectPressure() {
		pairsPerNS = 0
	}

	globalPair := 0
	for i := 1; i <= nsCount; i++ {
		ns := Namespace{
			Index: i,
			Name:  naming.SanitizeDNSLabel(factory.Name(naming.KindNamespace, i)),
		}
		for p := 1; p <= pairsPerNS; p++ {
			globalPair++
			app := naming.SanitizeDNSLabel(factory.Name(naming.KindPair, globalPair))
			ns.Pairs = append(ns.Pairs, Pair{
				Index:      p,
				App:        app,
				Service:    naming.SanitizeDNSLabel(factory.Name(naming.KindService, globalPair)),
				Route:      naming.SanitizeDNSLabel(factory.Name(naming.KindRoute, globalPair)),
				Deployment: naming.SanitizeDNSLabel(factory.Name(naming.KindDeployment, globalPair)),
				Replicas:   replicas,
			})
		}
		g.Namespaces = append(g.Namespaces, ns)
	}

	if len(g.Namespaces) != g.Counts.Namespaces {
		return nil, fmt.Errorf("namespace count mismatch: generated %d want %d", len(g.Namespaces), g.Counts.Namespaces)
	}
	return g, nil
}

func CommonLabels(runID, kind string) map[string]string {
	return map[string]string{
		LabelManaged: "true",
		LabelRun:     runID,
		LabelKind:    kind,
	}
}

func PairLabels(runID, app, kind string) map[string]string {
	labels := CommonLabels(runID, kind)
	labels[LabelApp] = app
	labels[LabelPair] = app
	return labels
}
