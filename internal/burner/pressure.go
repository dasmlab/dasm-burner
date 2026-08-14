package burner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

// WriteObjectPressureDir renders kube-burner init.yml + objectTemplates for ObjectPressure.
func WriteObjectPressureDir(outDir string, cfg *config.Config, g *topology.Graph, promURL, tokenFile, metricsDir string) (*Files, error) {
	files := &Files{
		Dir:             outDir,
		MeasureConfig:   filepath.Join(outDir, "measure.yml"),
		InitConfig:      filepath.Join(outDir, "init.yml"),
		MetricsProfile:  filepath.Join(outDir, "metrics.yml"),
		AlertsProfile:   filepath.Join(outDir, "alerts.yml"),
		MetricsEndpoint: filepath.Join(outDir, "metrics-endpoint.yml"),
		ObjectTemplates: filepath.Join(outDir, "objectTemplates"),
	}
	if err := os.MkdirAll(files.ObjectTemplates, 0o755); err != nil {
		return nil, err
	}
	if metricsDir == "" {
		metricsDir = filepath.Join(outDir, "collected")
	}
	if err := os.MkdirAll(metricsDir, 0o755); err != nil {
		return nil, err
	}

	measure := map[string]any{
		"metricsEndpoints": []map[string]any{{"indexer": localIndexer(metricsDir)}},
		"global":           map[string]any{"gc": false, "measurements": []any{}},
	}
	if err := writeYAML(files.MeasureConfig, measure); err != nil {
		return nil, err
	}

	qps := cfg.Deployment.APIConcurrency
	if qps < 1 {
		qps = 20
	}
	var objects []map[string]any
	for _, o := range cfg.Topology.Objects {
		if !o.Enabled {
			continue
		}
		fname, err := writePressureTemplate(files.ObjectTemplates, g.RunID, o)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{
			"objectTemplate": "objectTemplates/" + fname,
			"replicas":       o.ReplicasPerNS,
		}
		if o.WaitForReady {
			entry["wait"] = true
		}
		objects = append(objects, entry)
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("object pressure: no enabled objects")
	}

	initCfg := map[string]any{
		"global": map[string]any{
			"gc":               false,
			"waitWhenFinished": true,
		},
		"jobs": []map[string]any{
			{
				"name":                 "object-pressure",
				"namespace":            "kb-" + g.RunID,
				"jobIterations":        cfg.Topology.Namespaces.Count,
				"namespacedIterations": true,
				"podWait":              false,
				"qps":                  qps,
				"burst":                qps * 2,
				"namespaceLabels": map[string]string{
					topology.LabelManaged: "true",
					topology.LabelRun:     g.RunID,
					topology.LabelConfig:  cfg.Metadata.Name,
				},
				"objects": objects,
			},
		},
	}
	if err := writeYAML(files.InitConfig, initCfg); err != nil {
		return nil, err
	}
	if err := os.WriteFile(files.MetricsProfile, []byte(metricsProfile), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(files.AlertsProfile, []byte(alertsProfile), 0o644); err != nil {
		return nil, err
	}
	ep := []map[string]any{{
		"endpoint":      promURL,
		"skipTLSVerify": true,
		"metrics":       []string{files.MetricsProfile},
		"alerts":        []string{files.AlertsProfile},
		"indexer":       localIndexer(metricsDir),
	}}
	if tokenFile != "" {
		ep[0]["tokenFile"] = tokenFile
	}
	if err := writeYAML(files.MetricsEndpoint, ep); err != nil {
		return nil, err
	}
	return files, nil
}

func writePressureTemplate(dir, runID string, o config.PressureObject) (string, error) {
	safe := sanitizeTemplateName(o.ID)
	fname := safe + ".yml"
	path := filepath.Join(dir, fname)
	if strings.TrimSpace(o.InlineYAML) != "" {
		body := o.InlineYAML
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		return fname, os.WriteFile(path, []byte(body), 0o644)
	}
	ref := o.TemplateRef
	if ref == "" {
		ref = strings.ToLower(o.Kind)
	}
	body, ok := stockPressureTemplates[ref]
	if !ok {
		body = genericPressureTemplate(o)
	}
	body = strings.ReplaceAll(body, "__RUN__", runID)
	return fname, os.WriteFile(path, []byte(body), 0o644)
}

func sanitizeTemplateName(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "object"
	}
	return s
}

func genericPressureTemplate(o config.PressureObject) string {
	return fmt.Sprintf(`apiVersion: %s
kind: %s
metadata:
  name: kb-{{.Iteration}}-%s-{{.Replica}}
  labels:
    %s: "true"
    %s: "{{.UUID}}"
`, o.APIVersion, o.Kind, sanitizeTemplateName(o.ID), topology.LabelManaged, topology.LabelRun)
}

var stockPressureTemplates = map[string]string{
	"configmap": `apiVersion: v1
kind: ConfigMap
metadata:
  name: kb-{{.Iteration}}-cm-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
data:
  payload: "dasm-burner-object-pressure-{{.Iteration}}-{{.Replica}}"
`,
	"secret": `apiVersion: v1
kind: Secret
metadata:
  name: kb-{{.Iteration}}-sec-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
type: Opaque
stringData:
  token: "dasm-burner-{{.Iteration}}-{{.Replica}}"
`,
	"serviceaccount": `apiVersion: v1
kind: ServiceAccount
metadata:
  name: kb-{{.Iteration}}-sa-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
`,
	"rolebinding": `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kb-{{.Iteration}}-rb-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
  - kind: ServiceAccount
    name: default
`,
	"networkpolicy": `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kb-{{.Iteration}}-np-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
spec:
  podSelector: {}
  policyTypes: ["Ingress"]
`,
	"limitrange": `apiVersion: v1
kind: LimitRange
metadata:
  name: kb-{{.Iteration}}-lr-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
spec:
  limits:
    - type: Container
      default:
        memory: 64Mi
        cpu: 100m
      defaultRequest:
        memory: 32Mi
        cpu: 50m
`,
	"resourcequota": `apiVersion: v1
kind: ResourceQuota
metadata:
  name: kb-{{.Iteration}}-rq-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
spec:
  hard:
    pods: "200"
    requests.cpu: "20"
    requests.memory: 40Gi
`,
	"egressfirewall": `apiVersion: k8s.ovn.org/v1
kind: EgressFirewall
metadata:
  name: default
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "{{.UUID}}"
spec:
  egress:
    - type: Allow
      to:
        cidrSelector: 0.0.0.0/0
`,
}
