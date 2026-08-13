package ui

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/dasmlab/dasm-burner/internal/burner"
	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

// compactTopology is the canvas schema: one namespace box with instance
// counts, not N drawn namespaces. It maps onto kube-burner jobIterations /
// object replicas (see docs/KUBE-BURNER.md).
type compactTopology struct {
	Name                 string `json:"name"`
	Namespaces           int    `json:"namespaces"`
	RoutesPerNamespace   int    `json:"routesPerNamespace"`
	ServicesPerNamespace int    `json:"servicesPerNamespace"`
	ReplicasPerService   int    `json:"replicasPerService"`
	RouteToService       string `json:"routeToService"`
	Counts               any    `json:"counts,omitempty"`
	KubeBurner           any    `json:"kubeBurner,omitempty"`
	Warning              string `json:"warning,omitempty"`
	ActiveTemplate       string `json:"activeTemplate,omitempty"`
}

func compactFrom(cfg *config.Config) compactTopology {
	return compactTopology{
		Name:                 cfg.Metadata.Name,
		Namespaces:           cfg.Topology.Namespaces.Count,
		RoutesPerNamespace:   cfg.Topology.Routes.PerNamespace,
		ServicesPerNamespace: cfg.Topology.Services.PerNamespace,
		ReplicasPerService:   cfg.Topology.Workloads.ReplicasPerService,
		RouteToService:       cfg.Topology.Relationships.RouteToService,
		Counts:               cfg.Counts(),
		KubeBurner: map[string]any{
			"version":              burner.KubeBurnerVersion,
			"jobIterations":        cfg.Topology.Namespaces.Count,
			"namespacedIterations": true,
			"objectReplicas": map[string]int{
				"route":      cfg.Topology.Routes.PerNamespace,
				"service":    cfg.Topology.Services.PerNamespace,
				"deployment": cfg.Topology.Services.PerNamespace,
			},
			"deploymentReplicas": cfg.Topology.Workloads.ReplicasPerService,
			"templates":          []string{"{{.Iteration}}", "{{.Replica}}"},
		},
		Warning:        "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		ActiveTemplate: "",
	}
}

func (s *Server) topology(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.cfg()
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		out := compactFrom(cfg)
		out.ActiveTemplate = s.activeTemplateName()
		writeJSON(w, http.StatusOK, out)
	case http.MethodPut:
		// Save/overwrite active template (or create named one).
		s.saveTemplate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) kubeBurnerPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := s.cfg()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	g, err := topology.Generate(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dir := filepath.Join(s.RunDir, "preview-kube-burner")
	files, err := burner.WriteDir(dir, cfg, g, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	initYAML, err := os.ReadFile(files.InitConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  burner.KubeBurnerVersion,
		"initYaml": string(initYAML),
		"note":     "dasm-burner apply still uses client-go; this YAML is what kube-burner init would consume. Live runs use measure/index/check-alerts against the same go-templates.",
	})
}
