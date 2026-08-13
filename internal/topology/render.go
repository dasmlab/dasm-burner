package topology

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/dasmlab/dasm-burner/internal/config"
)

// Rendered is the YAML documents produced from a Graph.
type Rendered struct {
	Namespaces  []byte
	Services    []byte
	Routes      []byte
	Deployments []byte
}

func Render(g *Graph, cfg *config.Config) (*Rendered, error) {
	var nsBuf, svcBuf, rtBuf, depBuf bytes.Buffer
	for _, ns := range g.Namespaces {
		if err := writeDoc(&nsBuf, BuildNamespace(g, ns)); err != nil {
			return nil, err
		}
		for _, pair := range ns.Pairs {
			if err := writeDoc(&svcBuf, BuildService(g, ns, pair, cfg)); err != nil {
				return nil, err
			}
			if err := writeDoc(&rtBuf, BuildRoute(g, ns, pair, cfg)); err != nil {
				return nil, err
			}
			if err := writeDoc(&depBuf, BuildDeployment(g, ns, pair, cfg)); err != nil {
				return nil, err
			}
		}
	}
	return &Rendered{
		Namespaces:  nsBuf.Bytes(),
		Services:    svcBuf.Bytes(),
		Routes:      rtBuf.Bytes(),
		Deployments: depBuf.Bytes(),
	}, nil
}

func writeDoc(buf *bytes.Buffer, obj any) error {
	b, err := marshalClean(obj)
	if err != nil {
		return err
	}
	if buf.Len() > 0 {
		buf.WriteString("---\n")
	}
	buf.Write(b)
	return nil
}

func marshalClean(obj any) ([]byte, error) {
	b, err := yaml.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return b, nil
	}
	delete(m, "status")
	if md, ok := m["metadata"].(map[string]any); ok {
		delete(md, "creationTimestamp")
	}
	if spec, ok := m["spec"].(map[string]any); ok {
		delete(spec, "strategy")
		if tpl, ok := spec["template"].(map[string]any); ok {
			if tmd, ok := tpl["metadata"].(map[string]any); ok {
				delete(tmd, "creationTimestamp")
			}
		}
	}
	return yaml.Marshal(m)
}

// WriteRunDir writes the Phase 1 run envelope:
//
//	out/
//	  config.yaml
//	  rendered-config.yaml
//	  plan.json
//	  objects/namespaces.yaml
//	  objects/services.yaml
//	  objects/routes.yaml
//	  objects/deployments.yaml
func WriteRunDir(out string, srcPath string, cfg *config.Config, g *Graph) error {
	if err := os.MkdirAll(filepath.Join(out, "objects"), 0o755); err != nil {
		return err
	}

	if srcPath != "" {
		src, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("copy source config: %w", err)
		}
		if err := os.WriteFile(filepath.Join(out, "config.yaml"), src, 0o644); err != nil {
			return err
		}
	}

	renderedCfg, err := cfg.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "rendered-config.yaml"), renderedCfg, 0o644); err != nil {
		return err
	}

	plan, err := json.MarshalIndent(struct {
		RunID  string        `json:"runId"`
		Seed   int64         `json:"seed"`
		Counts config.Counts `json:"counts"`
		Graph  *Graph        `json:"graph"`
	}{g.RunID, g.Seed, g.Counts, g}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "plan.json"), plan, 0o644); err != nil {
		return err
	}

	docs, err := Render(g, cfg)
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"namespaces.yaml":  docs.Namespaces,
		"services.yaml":    docs.Services,
		"routes.yaml":      docs.Routes,
		"deployments.yaml": docs.Deployments,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(out, "objects", name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
