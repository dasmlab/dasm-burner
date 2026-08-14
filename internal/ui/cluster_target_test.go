package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClusterTargetValidateRemoteRequiresKubeconfig(t *testing.T) {
	t.Parallel()
	remote := clusterTarget{Name: "test-ovn-perf", Source: "login-command"}
	err := remote.validate()
	if err == nil {
		t.Fatal("expected error when remote has empty kubeconfig")
	}
	if got := err.Error(); !containsAll(got, "refusing silent in-cluster fallback") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClusterTargetValidateInClusterOK(t *testing.T) {
	t.Parallel()
	in := clusterTarget{Name: "2026-prod-1 (in-cluster)", Source: "in-cluster"}
	if err := in.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistAndRestoreSelectedCluster(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "remote.kubeconfig")
	if err := os.WriteFile(kc, []byte(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://api.test-ovn-perf.example:6443
contexts:
- name: test-ovn-perf
  context:
    cluster: c
    user: u
current-context: test-ovn-perf
users:
- name: u
  user:
    token: fake
`), 0o600); err != nil {
		t.Fatal(err)
	}

	s := &Server{RunDir: dir}
	want := clusterTarget{
		Name:       "test-ovn-perf",
		Source:     "login-command",
		Kubeconfig: kc,
		Context:    "test-ovn-perf",
		Server:     "https://api.test-ovn-perf.example:6443",
	}
	if err := s.persistSelectedCluster(want); err != nil {
		t.Fatal(err)
	}

	// Simulate fresh process: empty in-memory state, load from disk.
	s2 := &Server{RunDir: dir}
	cs := s2.clusterState()
	cs.mu.Lock()
	gotKC, gotCtx, gotSrc, gotName := cs.kubeconfig, cs.context, cs.source, cs.name
	cs.mu.Unlock()
	if gotKC != kc || gotCtx != "test-ovn-perf" || gotSrc != "login-command" || gotName != "test-ovn-perf" {
		t.Fatalf("restored state = kc=%q ctx=%q src=%q name=%q", gotKC, gotCtx, gotSrc, gotName)
	}

	snap, err := s2.snapshotTarget()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Name != "test-ovn-perf" || snap.Kubeconfig != kc || snap.isInCluster() {
		t.Fatalf("snapshot = %+v", snap)
	}
	line := snap.logLine()
	if !containsAll(line, "cluster=test-ovn-perf", "source=login-command", "server=https://api.test-ovn-perf.example:6443") {
		t.Fatalf("logLine = %q", line)
	}
}

func TestPersistMissingKubeconfigFallsBackInCluster(t *testing.T) {
	dir := t.TempDir()
	s := &Server{RunDir: dir}
	missing := filepath.Join(dir, "gone.kubeconfig")
	_ = s.persistSelectedCluster(clusterTarget{
		Name:       "ghost",
		Source:     "login-command",
		Kubeconfig: missing,
		Context:    "ghost",
	})

	s2 := &Server{RunDir: dir}
	cs := s2.clusterState()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// Missing file → treat as fresh default (in-cluster / empty).
	if cs.kubeconfig != "" || cs.context != "" {
		t.Fatalf("expected empty state when persisted kubeconfig missing, got kc=%q ctx=%q", cs.kubeconfig, cs.context)
	}
}

func TestSnapshotTargetRejectsRemoteWithoutPath(t *testing.T) {
	s := &Server{RunDir: t.TempDir()}
	cs := s.clusterState()
	cs.mu.Lock()
	cs.kubeconfig = ""
	cs.context = "test-ovn-perf"
	cs.source = "login-command"
	cs.name = "test-ovn-perf"
	cs.mu.Unlock()

	_, err := s.snapshotTarget()
	if err == nil {
		t.Fatal("expected snapshotTarget to fail for remote without kubeconfig path")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsStr(s, p) {
			return false
		}
	}
	return true
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(s) > 0 && stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
