package burner

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

const KubeBurnerVersion = "v2.8.1"

type Files struct {
	Dir             string
	MeasureConfig   string
	InitConfig      string
	MetricsProfile  string
	AlertsProfile   string
	MetricsEndpoint string
	ObjectTemplates string
}

// WriteDir renders kube-burner configs from a dasm-burner topology.
// Phase 2 still owns apply; kube-burner measure/index consume these files.
func WriteDir(outDir string, cfg *config.Config, g *topology.Graph, promURL, tokenFile, metricsDir string) (*Files, error) {
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
		"metricsEndpoints": []map[string]any{
			{
				"indexer": map[string]any{
					"type":             "local",
					"metricsDirectory": metricsDir,
				},
			},
		},
		"global": map[string]any{
			"gc":           false,
			"measurements": measurements(cfg),
		},
	}
	if err := writeYAML(files.MeasureConfig, measure); err != nil {
		return nil, err
	}

	qps := cfg.Deployment.APIConcurrency
	if qps < 1 {
		qps = 20
	}
	initCfg := map[string]any{
		"global": map[string]any{
			"gc":               false,
			"waitWhenFinished": true,
			"measurements":     measurements(cfg),
		},
		"jobs": []map[string]any{
			{
				"name":                 "route-service-density",
				"namespace":            "kb-" + g.RunID,
				"jobIterations":        cfg.Topology.Namespaces.Count,
				"namespacedIterations": true,
				"podWait":              cfg.Deployment.WaitForReady,
				"qps":                  qps,
				"burst":                qps * 2,
				"namespaceLabels": map[string]string{
					topology.LabelManaged: "true",
					topology.LabelRun:     g.RunID,
				},
				"objects": []map[string]any{
					{"objectTemplate": "objectTemplates/deployment.yml", "replicas": cfg.Topology.Services.PerNamespace},
					{"objectTemplate": "objectTemplates/service.yml", "replicas": cfg.Topology.Services.PerNamespace},
					{"objectTemplate": "objectTemplates/route.yml", "replicas": cfg.Topology.Routes.PerNamespace},
				},
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
		"indexer": map[string]any{
			"type":             "local",
			"metricsDirectory": metricsDir,
		},
	}}
	if tokenFile != "" {
		ep[0]["tokenFile"] = tokenFile
	}
	if err := writeYAML(files.MetricsEndpoint, ep); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(files.ObjectTemplates, "deployment.yml"),
		[]byte(initDeploymentTemplate(cfg, g.RunID)), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(files.ObjectTemplates, "service.yml"),
		[]byte(initServiceTemplate(cfg, g.RunID)), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(files.ObjectTemplates, "route.yml"),
		[]byte(initRouteTemplate(cfg, g.RunID)), 0o644); err != nil {
		return nil, err
	}

	readme := fmt.Sprintf("kube-burner %s configs for run %s (seed %d)\nMeasure with:\n  kube-burner measure -c measure.yml --selector=%s --duration=10m\n",
		KubeBurnerVersion, g.RunID, g.Seed, topology.Selector(g.RunID))
	_ = os.WriteFile(filepath.Join(outDir, "README.txt"), []byte(readme), 0o644)
	return files, nil
}

func measurements(cfg *config.Config) []map[string]any {
	var m []map[string]any
	if cfg.Monitoring.PodLatency.Enabled {
		m = append(m, map[string]any{"name": "podLatency"})
	}
	if cfg.Monitoring.ServiceLatency.Enabled {
		m = append(m, map[string]any{"name": "serviceLatency", "svcTimeout": "60s"})
	}
	if len(m) == 0 {
		m = []map[string]any{{"name": "podLatency"}}
	}
	return m
}

func writeYAML(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func initDeploymentTemplate(cfg *config.Config, runID string) string {
	pull := ""
	if cfg.Application.ImagePullSecret != "" {
		pull = "\n      imagePullSecrets:\n      - name: " + cfg.Application.ImagePullSecret
	}
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: kb-%[1]s-deploy-{{.Iteration}}-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: "%[1]s"
    app: kb-%[1]s-pair-{{.Iteration}}-{{.Replica}}
spec:
  replicas: %[2]d
  selector:
    matchLabels:
      app: kb-%[1]s-pair-{{.Iteration}}-{{.Replica}}
  template:
    metadata:
      labels:
        dasm-burner.dasmlab.org/managed: "true"
        dasm-burner.dasmlab.org/run: "%[1]s"
        app: kb-%[1]s-pair-{{.Iteration}}-{{.Replica}}
    spec:%[3]s
      containers:
      - name: web
        image: %[4]s
        ports:
        - name: http
          containerPort: %[5]d
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        readinessProbe:
          httpGet:
            path: /readyz
            port: http
        livenessProbe:
          httpGet:
            path: /healthz
            port: http
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          capabilities:
            drop: ["ALL"]
          seccompProfile:
            type: RuntimeDefault
`, runID, cfg.Topology.Workloads.ReplicasPerService, pull, cfg.Application.Image, cfg.Application.Port)
}

func initServiceTemplate(cfg *config.Config, runID string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: kb-%[1]s-svc-{{.Iteration}}-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: %[1]q
    app: kb-%[1]s-pair-{{.Iteration}}-{{.Replica}}
spec:
  selector:
    app: kb-%[1]s-pair-{{.Iteration}}-{{.Replica}}
  ports:
  - name: http
    port: %[2]d
    targetPort: http
`, runID, cfg.Application.Port)
}

func initRouteTemplate(cfg *config.Config, runID string) string {
	tls := ""
	if cfg.Application.TLS.Enabled {
		tls = fmt.Sprintf("  tls:\n    termination: %s\n    insecureEdgeTerminationPolicy: %s\n",
			cfg.Application.TLS.Termination, cfg.Application.TLS.InsecureEdgeTerminationPolicy)
	}
	return fmt.Sprintf(`apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: kb-%[1]s-rt-{{.Iteration}}-{{.Replica}}
  labels:
    dasm-burner.dasmlab.org/managed: "true"
    dasm-burner.dasmlab.org/run: %[1]q
    app: kb-%[1]s-pair-{{.Iteration}}-{{.Replica}}
spec:
  to:
    kind: Service
    name: kb-%[1]s-svc-{{.Iteration}}-{{.Replica}}
    weight: 100
  port:
    targetPort: http
%[2]s  wildcardPolicy: None
`, runID, tls)
}
