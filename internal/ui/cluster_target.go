package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dasmlab/dasm-burner/internal/kube"
)

// clusterTarget is a frozen Execute/Cleanup destination. Jobs must use this
// snapshot — never re-read a drifted global mid-run.
type clusterTarget struct {
	Name       string `json:"name"`
	Source     string `json:"source"` // in-cluster | kubeconfig | login-command | registry
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`
	Server     string `json:"server,omitempty"`
}

func (t clusterTarget) isInCluster() bool {
	return t.Source == "in-cluster" || (t.Kubeconfig == "" && t.Context == "" && t.Source != "kubeconfig" && t.Source != "login-command" && t.Source != "registry")
}

func (t clusterTarget) logLine() string {
	src := t.Source
	if src == "" {
		if t.isInCluster() {
			src = "in-cluster"
		} else {
			src = "kubeconfig"
		}
	}
	server := t.Server
	if server == "" {
		server = "(unknown)"
	}
	return fmt.Sprintf("cluster=%s source=%s server=%s", t.Name, src, server)
}

func (t clusterTarget) validate() error {
	if t.isInCluster() {
		return nil
	}
	if strings.TrimSpace(t.Kubeconfig) == "" {
		return fmt.Errorf("selected cluster %q (source=%s) has no kubeconfig path — refusing silent in-cluster fallback", t.Name, t.Source)
	}
	if _, err := os.Stat(t.Kubeconfig); err != nil {
		return fmt.Errorf("selected cluster %q kubeconfig %q unreadable: %w — refusing silent in-cluster fallback", t.Name, t.Kubeconfig, err)
	}
	return nil
}

func (t clusterTarget) client(qps float32, burst int) (kube.Cluster, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	if t.isInCluster() {
		return kube.NewLiveContext("", "", qps, burst)
	}
	return kube.NewLiveContext(t.Kubeconfig, t.Context, qps, burst)
}

func (s *Server) selectedClusterPath() string {
	return filepath.Join(s.RunDir, "selected-cluster.json")
}

func (s *Server) persistSelectedCluster(t clusterTarget) error {
	if s.RunDir == "" {
		return nil
	}
	_ = os.MkdirAll(s.RunDir, 0o755)
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.selectedClusterPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.selectedClusterPath())
}

func (s *Server) loadPersistedCluster() (clusterTarget, bool) {
	path := s.selectedClusterPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return clusterTarget{}, false
	}
	var t clusterTarget
	if err := json.Unmarshal(b, &t); err != nil {
		return clusterTarget{}, false
	}
	if t.Name == "" && t.Source == "" && t.Kubeconfig == "" {
		return clusterTarget{}, false
	}
	if !t.isInCluster() {
		if err := t.validate(); err != nil {
			return clusterTarget{}, false
		}
	}
	return t, true
}

// snapshotTarget freezes the currently selected cluster for a mutate job.
func (s *Server) snapshotTarget() (clusterTarget, error) {
	c := s.currentCluster()
	t := clusterTarget{
		Name:       c.Name,
		Source:     c.Source,
		Kubeconfig: c.Kubeconfig,
		Context:    c.Context,
		Server:     c.Server,
	}
	cs := s.clusterState()
	cs.mu.Lock()
	kc := cs.kubeconfig
	ctx := cs.context
	src := cs.source
	selName := cs.name
	cs.mu.Unlock()

	// Prefer explicit in-memory selection over list display quirks.
	if src == "in-cluster" || (kc == "" && ctx == "" && (src == "" || src == "in-cluster")) {
		t.Source = "in-cluster"
		t.Kubeconfig = ""
		t.Context = ""
		if t.Name == "" {
			t.Name = envOr("CLUSTER_DISPLAY_NAME", "in-cluster")
		}
		if host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST")); host != "" && t.Server == "" {
			t.Server = "https://" + host
		}
	} else if kc != "" {
		t.Kubeconfig = kc
		t.Context = ctx
		if src != "" {
			t.Source = src
		}
		if t.Source == "" {
			t.Source = "kubeconfig"
		}
		if selName != "" {
			t.Name = selName
		}
	} else if src == "login-command" || src == "kubeconfig" || src == "registry" {
		// Remote was selected but path is missing — fail in validate(), never fall through to in-cluster.
		t.Source = src
		t.Kubeconfig = ""
		t.Context = ctx
		if selName != "" {
			t.Name = selName
		}
	} else if s.Kubeconfig != "" && t.Source != "in-cluster" {
		// Local serve --kubeconfig with no UI selection yet.
		t.Kubeconfig = s.Kubeconfig
		if t.Source == "" {
			t.Source = "kubeconfig"
		}
	}

	if err := t.validate(); err != nil {
		return t, err
	}
	return t, nil
}
