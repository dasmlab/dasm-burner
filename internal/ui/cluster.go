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
}

func (s *Server) clusterState() *clusterState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clusters == nil {
		s.clusters = &clusterState{kubeconfig: s.Kubeconfig}
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
		writeJSON(w, http.StatusOK, map[string]any{
			"current":  s.currentCluster(),
			"clusters": s.listClusters(),
			"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
		})
	case http.MethodPut:
		s.selectCluster(w, r)
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
	cs.kubeconfig = pick.Kubeconfig
	cs.context = pick.Context
	cs.mu.Unlock()
	if pick.Context != "" && pick.Kubeconfig != "" {
		_ = os.Setenv("KUBECONFIG", pick.Kubeconfig)
		// Prefer explicit context via loading rules in NewLive; store for display.
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"current": s.currentCluster(),
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
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
