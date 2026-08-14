package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/client-go/tools/clientcmd"
)

type clusterInfo struct {
	Name       string `json:"name"`
	Server     string `json:"server,omitempty"`
	User       string `json:"user,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Source     string `json:"source"` // in-cluster | kubeconfig
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`
	Current    bool   `json:"current"`
	Warning    string `json:"warning,omitempty"`
}

type clusterState struct {
	mu         sync.Mutex
	kubeconfig string
	context    string
	source     string // in-cluster | kubeconfig | login-command | registry
	name       string
}

func (s *Server) clusterState() *clusterState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clusters == nil {
		s.clusters = &clusterState{kubeconfig: s.Kubeconfig}
		if t, ok := s.loadPersistedCluster(); ok {
			if t.isInCluster() {
				s.clusters.kubeconfig = ""
				s.clusters.context = ""
				s.clusters.source = "in-cluster"
				s.clusters.name = t.Name
			} else {
				s.clusters.kubeconfig = t.Kubeconfig
				s.clusters.context = t.Context
				s.clusters.source = t.Source
				s.clusters.name = t.Name
			}
		} else if s.Kubeconfig == "" {
			// Pod default: explicit in-cluster until the operator selects a remote.
			s.clusters.source = "in-cluster"
		}
	}
	return s.clusters
}

func (s *Server) activeKubeconfig() string {
	cs := s.clusterState()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.kubeconfig != "" {
		return cs.kubeconfig
	}
	return s.Kubeconfig
}

func (s *Server) cluster(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.listClusters()
		cur := s.currentCluster()
		if !s.requesterIsAdmin(r) {
			list = redactClusterSecrets(list)
			cur = redactClusterInfo(cur)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"current":  cur,
			"clusters": list,
			"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		})
	case http.MethodPut:
		s.selectCluster(w, r)
	case http.MethodDelete:
		s.deleteCluster(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) selectCluster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		Kubeconfig string `json:"kubeconfig"`
		Context    string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	list := s.listClusters()
	var pick *clusterInfo
	for i := range list {
		c := &list[i]
		if body.Name != "" && c.Name == body.Name {
			pick = c
			break
		}
		if body.Context != "" && c.Context == body.Context && (body.Kubeconfig == "" || body.Kubeconfig == c.Kubeconfig) {
			pick = c
			break
		}
	}
	if pick == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown cluster context"))
		return
	}
	cs := s.clusterState()
	cs.mu.Lock()
	if pick.Source == "in-cluster" {
		cs.kubeconfig = ""
		cs.context = ""
		cs.source = "in-cluster"
		cs.name = pick.Name
	} else {
		cs.kubeconfig = pick.Kubeconfig
		cs.context = pick.Context
		cs.source = pick.Source
		cs.name = pick.Name
	}
	persisted := clusterTarget{
		Name:       pick.Name,
		Source:     pick.Source,
		Kubeconfig: pick.Kubeconfig,
		Context:    pick.Context,
		Server:     pick.Server,
	}
	if pick.Source == "in-cluster" {
		persisted.Kubeconfig = ""
		persisted.Context = ""
	}
	cs.mu.Unlock()
	_ = s.persistSelectedCluster(persisted)
	writeJSON(w, http.StatusOK, map[string]any{
		"current": s.currentCluster(),
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func redactClusterInfo(c clusterInfo) clusterInfo {
	c.Kubeconfig = ""
	c.User = ""
	return c
}

func redactClusterSecrets(list []clusterInfo) []clusterInfo {
	out := make([]clusterInfo, len(list))
	for i, c := range list {
		out[i] = redactClusterInfo(c)
	}
	return out
}

func (s *Server) deleteCluster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	list := s.listClusters()
	var pick *clusterInfo
	for i := range list {
		if list[i].Name == name {
			pick = &list[i]
			break
		}
	}
	if pick == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown cluster"))
		return
	}
	if pick.Source != "login-command" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("only token-added clusters can be removed (not in-cluster or file kubeconfig contexts)"))
		return
	}
	dir, err := filepath.Abs(s.clustersDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	abs, err := filepath.Abs(pick.Kubeconfig)
	if err != nil || abs == "" || !strings.HasPrefix(abs, dir+string(os.PathSeparator)) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("refusing to delete kubeconfig outside clusters dir"))
		return
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if pick.Current {
		cs := s.clusterState()
		cs.mu.Lock()
		cs.kubeconfig = ""
		cs.context = ""
		cs.source = "in-cluster"
		cs.name = envOr("CLUSTER_DISPLAY_NAME", envOr("CLUSTER_NAME", "in-cluster"))
		name := cs.name
		cs.mu.Unlock()
		_ = s.persistSelectedCluster(clusterTarget{Name: name, Source: "in-cluster"})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":  name,
		"current":  s.currentCluster(),
		"clusters": s.listClusters(),
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) currentCluster() clusterInfo {
	list := s.listClusters()
	for _, c := range list {
		if c.Current {
			return c
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return clusterInfo{
		Name:    "unknown",
		Source:  "in-cluster",
		Warning: "no kubeconfig contexts found; using in-cluster SA",
	}
}

func (s *Server) listClusters() []clusterInfo {
	cs := s.clusterState()
	cs.mu.Lock()
	activeKC := cs.kubeconfig
	activeCtx := cs.context
	cs.mu.Unlock()
	if activeKC == "" {
		activeKC = s.Kubeconfig
	}

	var out []clusterInfo
	seen := map[string]bool{}

	// In-cluster always available when serving from a pod.
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	if host != "" {
		name := envOr("CLUSTER_NAME", "in-cluster")
		if name == "in-cluster" {
			if v := strings.TrimSpace(os.Getenv("CLUSTER_DISPLAY_NAME")); v != "" {
				name = v
			} else {
				name = "2026-prod-1 (in-cluster)"
			}
		}
		c := clusterInfo{
			Name:      name,
			Server:    "https://" + host,
			User:      "serviceaccount:dasm-burner-sa",
			Source:    "in-cluster",
			Current:   activeKC == "" && activeCtx == "",
			Warning:   "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
			Namespace: "dasm-burner-system",
		}
		out = append(out, c)
		seen[c.Name] = true
	}

	paths := kubeconfigPaths(activeKC)
	for _, path := range paths {
		cfg, err := clientcmd.LoadFromFile(path)
		if err != nil || cfg == nil {
			continue
		}
		current := cfg.CurrentContext
		for name, ctx := range cfg.Contexts {
			if ctx == nil {
				continue
			}
			clusterName := ctx.Cluster
			server := ""
			if cl, ok := cfg.Clusters[clusterName]; ok && cl != nil {
				server = cl.Server
			}
			display := name
			if clusterName != "" && clusterName != name {
				display = name + " · " + clusterName
			}
			info := clusterInfo{
				Name:       display,
				Server:     server,
				User:       ctx.AuthInfo,
				Namespace:  ctx.Namespace,
				Source:     "kubeconfig",
				Kubeconfig: path,
				Context:    name,
				Warning:    "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
			}
			if activeCtx != "" {
				info.Current = activeCtx == name && (activeKC == "" || activeKC == path)
			} else if activeKC != "" {
				info.Current = activeKC == path && name == current
			} else {
				info.Current = name == current && host == ""
			}
			key := path + "|" + name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, info)
		}
	}

	// Optional explicit registry: CLUSTER_CONTEXTS=/path/a.kubeconfig:/path/b.kubeconfig
	if raw := strings.TrimSpace(os.Getenv("CLUSTER_CONTEXTS")); raw != "" {
		for _, p := range strings.Split(raw, ":") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			abs := p
			if !filepath.IsAbs(abs) {
				abs, _ = filepath.Abs(p)
			}
			cfg, err := clientcmd.LoadFromFile(abs)
			if err != nil {
				continue
			}
			for name, ctx := range cfg.Contexts {
				if ctx == nil {
					continue
				}
				server := ""
				if cl, ok := cfg.Clusters[ctx.Cluster]; ok && cl != nil {
					server = cl.Server
				}
				info := clusterInfo{
					Name:       name,
					Server:     server,
					User:       ctx.AuthInfo,
					Namespace:  ctx.Namespace,
					Source:     "registry",
					Kubeconfig: abs,
					Context:    name,
					Current:    activeCtx == name && activeKC == abs,
					Warning:    "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
				}
				key := abs + "|" + name
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, info)
			}
		}
	}

	// Login-command targets written under runDir/clusters/*.kubeconfig
	if entries, err := os.ReadDir(s.clustersDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".kubeconfig") {
				continue
			}
			path := filepath.Join(s.clustersDir(), e.Name())
			cfg, err := clientcmd.LoadFromFile(path)
			if err != nil || cfg == nil {
				continue
			}
			for name, ctx := range cfg.Contexts {
				if ctx == nil {
					continue
				}
				server := ""
				if cl, ok := cfg.Clusters[ctx.Cluster]; ok && cl != nil {
					server = cl.Server
				}
				display := name
				info := clusterInfo{
					Name:       display,
					Server:     server,
					User:       "token",
					Namespace:  ctx.Namespace,
					Source:     "login-command",
					Kubeconfig: path,
					Context:    name,
					Current:    activeCtx == name && activeKC == path,
					Warning:    "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
				}
				key := path + "|" + name
				if seen[key] || seen[display] {
					// Prefer updating current flag if name collision with in-cluster label.
					continue
				}
				seen[key] = true
				seen[display] = true
				out = append(out, info)
			}
		}
	}

	return out
}

func kubeconfigPaths(primary string) []string {
	var paths []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, existing := range paths {
			if existing == p {
				return
			}
		}
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	add(primary)
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		for _, p := range filepath.SplitList(kc) {
			add(p)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".kube", "config"))
	}
	add("/var/run/secrets/kubernetes.io/serviceaccount") // not a kubeconfig; ignored by Stat of file — skip
	return paths
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
