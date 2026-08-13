package ui

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dasmlab/dasm-burner/internal/auth"
	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

type Server struct {
	Mux            *http.ServeMux
	Version        string
	RunDir         string
	ConfigPath     string
	Kubeconfig     string
	auth           *auth.Service
	mu             sync.Mutex
	static         http.Handler
	clusters       *clusterState
	activeTemplate string
	exec           *execManager
}

func New(version, runDir, configPath, kubeconfig string, static fs.FS, authSvc *auth.Service) *Server {
	s := &Server{
		Mux:        http.NewServeMux(),
		Version:    version,
		RunDir:     runDir,
		ConfigPath: configPath,
		Kubeconfig: kubeconfig,
		auth:       authSvc,
	}
	if static != nil {
		s.static = StaticFS{Root: http.FS(static)}
	}
	s.ensureDefaultTemplates()
	s.Mux.HandleFunc("/healthz", s.ok)
	s.Mux.HandleFunc("/readyz", s.ok)
	s.Mux.HandleFunc("/api/v1/version", s.version)
	s.Mux.HandleFunc("/api/v1/auth/config", s.authConfig)
	if s.auth != nil {
		s.Mux.HandleFunc("/api/v1/auth/login", s.auth.Login)
		s.Mux.HandleFunc("/api/v1/auth/callback", s.auth.Callback)
		s.Mux.HandleFunc("/api/v1/auth/logout", s.auth.Logout)
		s.Mux.HandleFunc("/api/v1/auth/me", s.auth.Me)
		s.Mux.HandleFunc("/api/v1/auth/keepalive", s.auth.KeepAlive)
	}
	s.Mux.Handle("/api/v1/plan", s.protect(s.plan))
	s.Mux.Handle("/api/v1/status", s.protect(s.status))
	s.Mux.Handle("/api/v1/health", s.protect(s.health))
	s.Mux.Handle("/api/v1/health/baseline", s.protect(s.healthBaselineAPI))
	s.Mux.Handle("/api/v1/overview", s.protect(s.overview))
	s.Mux.Handle("/api/v1/report", s.protect(s.report))
	s.Mux.Handle("/api/v1/reports", s.protect(s.reports))
	s.Mux.Handle("/api/v1/reports/", s.protect(s.reportByID))
	s.Mux.Handle("/api/v1/topology", s.protect(s.topology))
	s.Mux.Handle("/api/v1/templates", s.protect(s.templates))
	s.Mux.Handle("/api/v1/templates/", s.protect(s.templateByName))
	s.Mux.Handle("/api/v1/cluster", s.protect(s.cluster))
	s.Mux.Handle("/api/v1/cluster/login", s.protect(s.addClusterLogin))
	s.Mux.Handle("/api/v1/runs", s.protect(s.runs))
	s.Mux.Handle("/api/v1/runs/", s.protect(s.runAction))
	s.Mux.Handle("/api/v1/cleanup", s.protect(s.cleanupAPI))
	s.Mux.Handle("/api/v1/cleanup/check", s.protect(s.cleanupCheck))
	s.Mux.Handle("/api/v1/kube-burner-preview", s.protect(s.kubeBurnerPreview))
	s.Mux.HandleFunc("/", s.spa)
	return s
}

func (s *Server) protect(h http.HandlerFunc) http.Handler {
	if s.auth == nil {
		return h
	}
	return s.auth.AdminMiddleware(h)
}

func (s *Server) authConfig(w http.ResponseWriter, _ *http.Request) {
	if s.auth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, s.auth.ConfigInfo())
}

func (s *Server) ok(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.Version, "app": "dasm-burner"})
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{
		"runId":   g.RunID,
		"seed":    g.Seed,
		"name":    cfg.Metadata.Name,
		"counts":  g.Counts,
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
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
	cl, err := s.liveClient(20, 40)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	ctx := r.Context()
	snap, err := cl.ListManaged(ctx, g.RunID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runId":       g.RunID,
		"convergence": kube.ComputeConvergence(g.Counts, snap),
		"sampledAt":   time.Now(),
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
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
	cl, err := s.liveClient(20, 40)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	h, err := cl.ClusterHealth(r.Context(), g.RunID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	d := runner.Evaluate(cfg.Safety, h)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if s.static == nil {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" || !strings.Contains(path, ".") {
		r = r.Clone(r.Context())
		r.URL.Path = "/"
	}
	s.static.ServeHTTP(w, r)
}

func (s *Server) cfg() (*config.Config, error) {
	if name := s.activeTemplateName(); name != "" {
		if c, err := s.loadTemplate(name); err == nil {
			return c, nil
		}
	}
	s.mu.Lock()
	path := s.ConfigPath
	s.mu.Unlock()
	if path == "" {
		c := config.StartingTemplate()
		return c, config.Validate(c)
	}
	return config.Load(path)
}

func (s *Server) liveClient(qps float32, burst int) (kube.Cluster, error) {
	cs := s.clusterState()
	cs.mu.Lock()
	kc := cs.kubeconfig
	ctxName := cs.context
	cs.mu.Unlock()
	if kc == "" {
		kc = s.Kubeconfig
	}
	return kube.NewLiveContext(kc, ctxName, qps, burst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

type StaticFS struct {
	Root http.FileSystem
}

func (s StaticFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" || !strings.Contains(path, ".") {
		path = "index.html"
	}
	f, err := s.Root.Open(path)
	if err != nil {
		f, err = s.Root.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path = "index.html"
	}
	defer f.Close()
	mod := time.Time{}
	if st, err := f.Stat(); err == nil {
		mod = st.ModTime()
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, f)
		return
	}
	http.ServeContent(w, r, path, mod, rs)
}
