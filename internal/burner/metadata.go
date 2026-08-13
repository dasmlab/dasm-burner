package burner

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const MetricsTarballName = "kube-burner-metrics.tgz"

// UserMetadata is merged into kube-burner jobSummary via --user-metadata.
type UserMetadata struct {
	RunID             string `yaml:"runId" json:"runId"`
	Prefix            string `yaml:"prefix" json:"prefix"`
	Cluster           string `yaml:"cluster" json:"cluster"`
	Template          string `yaml:"template" json:"template"`
	DasmBurnerVersion string `yaml:"dasmBurnerVersion" json:"dasmBurnerVersion"`
	DryRun            bool   `yaml:"dryRun" json:"dryRun"`
	Namespaces        int    `yaml:"namespaces" json:"namespaces"`
	Services          int    `yaml:"services" json:"services"`
	Routes            int    `yaml:"routes" json:"routes"`
	Deployments       int    `yaml:"deployments" json:"deployments"`
	Pods              int    `yaml:"pods" json:"pods"`
}

// WriteUserMetadata writes kube-burner/user-metadata.yml under outDir.
func WriteUserMetadata(outDir string, meta UserMetadata) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(outDir, "user-metadata.yml")
	b, err := yaml.Marshal(meta)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func localIndexer(metricsDir string) map[string]any {
	return map[string]any{
		"type":             "local",
		"metricsDirectory": metricsDir,
		"createTarball":    true,
		"tarballName":      MetricsTarballName,
	}
}

func FormatPrefix(runID string) string {
	if runID == "" {
		return "kb-?"
	}
	return fmt.Sprintf("kb-%s", runID)
}
