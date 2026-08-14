package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/naming"
)

var templateNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

type templateMeta struct {
	Name                 string    `json:"name"`
	Description          string    `json:"description,omitempty"`
	Kind                 string    `json:"kind,omitempty"`
	Namespaces           int       `json:"namespaces"`
	RoutesPerNamespace   int       `json:"routesPerNamespace"`
	ServicesPerNamespace int       `json:"servicesPerNamespace"`
	ReplicasPerService   int       `json:"replicasPerService"`
	RouteToService       string    `json:"routeToService"`
	Objects              any       `json:"objects,omitempty"`
	Counts               any       `json:"counts,omitempty"`
	Prefix               string    `json:"prefix,omitempty"`
	RunID                string    `json:"runId,omitempty"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Active               bool      `json:"active,omitempty"`
}

func (s *Server) templatesDir() string {
	dir := filepath.Join(s.RunDir, "templates")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func (s *Server) activeTemplateName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTemplate
}

func (s *Server) setActiveTemplate(name string) {
	s.mu.Lock()
	s.activeTemplate = name
	if name != "" {
		s.ConfigPath = filepath.Join(s.templatesDir(), name+".yaml")
	}
	s.mu.Unlock()
}

func (s *Server) templates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"templates": s.listTemplates(),
			"active":    s.activeTemplateName(),
		})
	case http.MethodPost:
		s.saveTemplate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) templateByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/templates/")
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.loadTemplate(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		out := compactFrom(cfg)
		writeJSON(w, http.StatusOK, out)
	case http.MethodPut:
		// Select this saved template as active (Execute / Topology dropdown).
		if _, err := s.loadTemplate(name); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		s.setActiveTemplate(name)
		cfg, _ := s.loadTemplate(name)
		writeJSON(w, http.StatusOK, map[string]any{
			"active":   name,
			"topology": compactFrom(cfg),
		})
	case http.MethodDelete:
		path := filepath.Join(s.templatesDir(), name+".yaml")
		if err := os.Remove(path); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if s.activeTemplateName() == name {
			s.setActiveTemplate("")
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) saveTemplate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                 string                  `json:"name"`
		SaveAs               string                  `json:"saveAs"`
		Description          string                  `json:"description"`
		Kind                 string                  `json:"kind"`
		Namespaces           int                     `json:"namespaces"`
		RoutesPerNamespace   int                     `json:"routesPerNamespace"`
		ServicesPerNamespace int                     `json:"servicesPerNamespace"`
		ReplicasPerService   int                     `json:"replicasPerService"`
		RouteToService       string                  `json:"routeToService"`
		Objects              []config.PressureObject `json:"objects"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(body.SaveAs)
	if name == "" {
		name = strings.TrimSpace(body.Name)
	}
	if !templateNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("template name must be 1-64 chars [A-Za-z0-9._-]"))
		return
	}
	cfg, err := s.cfg()
	if err != nil {
		cfg = config.StartingTemplate()
	}
	if body.Kind == config.KindObjectPressure {
		cfg = config.StartingObjectPressure()
	} else if body.Kind == config.Kind || body.Kind == "" {
		if cfg.IsObjectPressure() && body.Kind == config.Kind {
			cfg = config.StartingTemplate()
		}
	}
	if body.Kind != "" {
		cfg.Kind = body.Kind
	}
	cfg.Metadata.Name = name
	if strings.TrimSpace(body.SaveAs) != "" {
		cfg.Naming.Seed = config.Seed{Auto: false, Value: config.SeedFromTemplateName(name)}
	} else {
		config.EnsureDistinctTemplateSeed(cfg)
	}
	if body.Description != "" {
		cfg.Metadata.Description = body.Description
	}
	if body.Namespaces > 0 {
		cfg.Topology.Namespaces.Count = body.Namespaces
	}
	if body.RoutesPerNamespace > 0 {
		cfg.Topology.Routes.PerNamespace = body.RoutesPerNamespace
	}
	if body.ServicesPerNamespace > 0 {
		cfg.Topology.Services.PerNamespace = body.ServicesPerNamespace
	}
	if body.ReplicasPerService > 0 {
		cfg.Topology.Workloads.ReplicasPerService = body.ReplicasPerService
	}
	if body.RouteToService != "" {
		cfg.Topology.Relationships.RouteToService = body.RouteToService
	}
	if body.Objects != nil {
		cfg.Topology.Objects = body.Objects
	}
	config.ApplyDefaults(cfg)
	if err := config.Validate(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	raw, err := cfg.Marshal()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	path := filepath.Join(s.templatesDir(), name+".yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.setActiveTemplate(name)
	writeJSON(w, http.StatusOK, map[string]any{
		"saved":     name,
		"prefix":    naming.PrefixFor(cfg.Naming),
		"runId":     naming.NewFactory(cfg.Naming).RunID(),
		"topology":  compactFrom(cfg),
		"templates": s.listTemplates(),
	})
}

func (s *Server) loadTemplate(name string) (*config.Config, error) {
	if !templateNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid template name")
	}
	path := filepath.Join(s.templatesDir(), name+".yaml")
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	changed := false
	if cfg.Metadata.Name != name {
		cfg.Metadata.Name = name
		changed = true
	}
	if config.EnsureDistinctTemplateSeed(cfg) {
		changed = true
	}
	if changed {
		if raw, merr := cfg.Marshal(); merr == nil {
			_ = os.WriteFile(path, raw, 0o644)
		}
	}
	return cfg, nil
}

func (s *Server) listTemplates() []templateMeta {
	dir := s.templatesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	active := s.activeTemplateName()
	var out []templateMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		cfg, err := config.Load(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		changed := false
		if cfg.Metadata.Name != name {
			cfg.Metadata.Name = name
			changed = true
		}
		if config.EnsureDistinctTemplateSeed(cfg) {
			changed = true
		}
		if changed {
			if raw, merr := cfg.Marshal(); merr == nil {
				_ = os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644)
			}
		}
		st, _ := e.Info()
		updated := time.Now()
		if st != nil {
			updated = st.ModTime()
		}
		out = append(out, templateMeta{
			Name:                 name,
			Description:          cfg.Metadata.Description,
			Kind:                 cfg.Kind,
			Namespaces:           cfg.Topology.Namespaces.Count,
			RoutesPerNamespace:   cfg.Topology.Routes.PerNamespace,
			ServicesPerNamespace: cfg.Topology.Services.PerNamespace,
			ReplicasPerService:   cfg.Topology.Workloads.ReplicasPerService,
			RouteToService:       cfg.Topology.Relationships.RouteToService,
			Objects:              cfg.Topology.Objects,
			Counts:               cfg.Counts(),
			Prefix:               naming.PrefixFor(cfg.Naming),
			RunID:                naming.NewFactory(cfg.Naming).RunID(),
			UpdatedAt:            updated,
			Active:               name == active,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Server) ensureDefaultTemplates() {
	dir := s.templatesDir()
	writeIfMissing := func(name string, cfg *config.Config) {
		path := filepath.Join(dir, name+".yaml")
		if _, err := os.Stat(path); err == nil {
			return
		}
		raw, err := cfg.Marshal()
		if err != nil {
			return
		}
		_ = os.WriteFile(path, raw, 0o644)
	}
	writeIfMissing("smoke", config.StartingTemplate())
	writeIfMissing("object-pressure", config.StartingObjectPressure())
	if s.activeTemplateName() == "" {
		s.setActiveTemplate("smoke")
	}
}
