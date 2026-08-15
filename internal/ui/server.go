package ui

import (
	"context"
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
	"github.com/dasmlab/dasm-burner/internal/ovndiag"
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
	cleanupBusy    bool
	ovnBase        *ovndiag.Baseline
	bus            *EventBus
	ovnQ           chan ovnJob
	ovnMu          sync.Mutex
	etcdQ          chan etcdJob
	etcdMu         sync.Mutex
	indexMu        sync.Mutex
	indexAt        time.Time
	indexCluster   string
	indexMetas     []nsMeta
	indexErr       error
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
	s.Mux.Handle("/api/v1/plan", s.admin(s.plan))
	s.Mux.Handle("/api/v1/status", s.guest(s.status))
	s.Mux.Handle("/api/v1/health", s.guest(s.health))
	s.Mux.Handle("/api/v1/health/baseline", s.allowGuest(s.healthBaselineAPI, http.MethodGet))
	s.Mux.Handle("/api/v1/overview", s.guest(s.overview))
	s.Mux.Handle("/api/v1/report", s.guest(s.report))
	s.Mux.Handle("/api/v1/reports", s.guest(s.reports))
	s.Mux.Handle("/api/v1/reports/", s.guest(s.reportByID))
	s.Mux.Handle("/api/v1/topology", s.allowGuest(s.topology, http.MethodGet))
	s.Mux.Handle("/api/v1/templates", s.allowGuest(s.templates, http.MethodGet))
	s.Mux.Handle("/api/v1/templates/", s.allowGuest(s.templateByName, http.MethodGet, http.MethodPut))
	s.Mux.Handle("/api/v1/cluster", s.allowGuest(s.cluster, http.MethodGet, http.MethodPut))
	s.Mux.Handle("/api/v1/cluster/login", s.admin(s.addClusterLogin))
	s.Mux.Handle("/api/v1/cluster/capacity", s.admin(s.clusterCapacityAPI))
	s.Mux.Handle("/api/v1/cluster/maxpods", s.admin(s.clusterMaxPodsAPI))
	s.Mux.Handle("/api/v1/events", s.guest(s.eventsAPI))
	s.Mux.Handle("/api/v1/runs", s.allowGuest(s.runs, http.MethodGet))
	s.Mux.Handle("/api/v1/runs/", s.allowGuest(s.runAction, http.MethodGet))
	s.Mux.Handle("/api/v1/cleanup", s.allowGuest(s.cleanupAPI, http.MethodGet))
	s.Mux.Handle("/api/v1/cleanup/check", s.guest(s.cleanupCheck))
	s.Mux.Handle("/api/v1/cleanup-reports", s.guest(s.cleanupReportsAPI))
	s.Mux.Handle("/api/v1/cleanup-reports/", s.guest(s.cleanupReportByID))
	s.Mux.Handle("/api/v1/kube-burner-preview", s.guest(s.kubeBurnerPreview))
	s.Mux.Handle("/api/v1/ovndiag", s.allowGuest(s.ovndiagAPI, http.MethodGet))
	s.Mux.Handle("/api/v1/ovndiag/baseline", s.admin(s.ovndiagBaselineAPI))
	s.Mux.Handle("/api/v1/ovndiag/sample", s.admin(s.ovndiagSample))
	s.Mux.Handle("/api/v1/ovndiag/history", s.guest(s.ovndiagHistoryAPI))
	s.Mux.Handle("/api/v1/ovndiag/history/", s.guest(s.ovndiagHistoryAPI))
	s.Mux.Handle("/api/v1/etcddiag", s.allowGuest(s.etcddiagAPI, http.MethodGet))
	s.Mux.Handle("/api/v1/etcddiag/baseline", s.admin(s.etcddiagBaselineAPI))
	s.Mux.Handle("/api/v1/etcddiag/sample", s.admin(s.etcddiagSample))
	s.Mux.Handle("/api/v1/etcddiag/history", s.guest(s.etcddiagHistoryAPI))
	s.Mux.Handle("/api/v1/etcddiag/history/", s.guest(s.etcddiagHistoryAPI))
	s.Mux.HandleFunc("/", s.spa)
	s.bus = NewEventBus()
	s.ovnQ = make(chan ovnJob, 4)
	go s.loopOVNWorker()
	s.etcdQ = make(chan etcdJob, 4)
	go s.loopEtcdWorker()
	go s.loopStatusPublisher()
	return s
}

func (s *Server) guest(h http.HandlerFunc) http.Handler {
	if s.auth == nil {
		return h
	}
	return s.auth.GuestMiddleware(h)
}

func (s *Server) admin(h http.HandlerFunc) http.Handler {
	if s.auth == nil {
		return h
	}
	return s.auth.AdminMiddleware(h)
}

// allowGuest lets the listed methods through as a viewer; everything else requires admin.
func (s *Server) allowGuest(h http.HandlerFunc, methods ...string) http.Handler {
	ok := map[string]bool{}
	for _, m := range methods {
		ok[m] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok[r.Method] {
			s.guest(h).ServeHTTP(w, r)
			return
		}
		s.admin(h).ServeHTTP(w, r)
	})
}

func (s *Server) requesterIsAdmin(r *http.Request) bool {
	if s.auth == nil || !s.auth.Enabled() {
		return true
	}
	u, ok := auth.UserFromContext(r.Context())
	return ok && u.IsAdmin
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
	plan := runner.PlanBatches(cfg, g.Namespaces)
	writeJSON(w, http.StatusOK, map[string]any{
		"runId":     g.RunID,
		"seed":      g.Seed,
		"name":      cfg.Metadata.Name,
		"counts":    g.Counts,
		"batchPlan": plan,
		"warning":   "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
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
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	names, err := cl.ListManagedNamespaces(ctx, g.RunID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runId":     g.RunID,
		"namespaces": len(names),
		"sampledAt": time.Now(),
		"note":      "counts are namespace-only; object tallies are not listed on the request path",
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
	t, err := s.snapshotTarget()
	if err != nil {
		return nil, err
	}
	return t.client(qps, burst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	if ce, ok := err.(*kube.CapacityExceededError); ok {
		writeJSON(w, status, map[string]any{
			"error":    ce.Error(),
			"code":     "capacity_exceeded",
			"capacity": ce.Capacity,
		})
		return
	}
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
