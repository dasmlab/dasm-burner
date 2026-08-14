package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	s := New("dev", t.TempDir(), "", "", nil, nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestVersion(t *testing.T) {
	s := New("vtest", t.TempDir(), "", "", nil, nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))
	if rec.Code != 200 || rec.Body.String() == "" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthConfigDisabled(t *testing.T) {
	s := New("dev", t.TempDir(), "", "", nil, nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/config", nil))
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestTemplateRoundTrip(t *testing.T) {
	s := New("dev", t.TempDir(), "", "", nil, nil)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil))
	if rec.Code != 200 {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"smoke"`) {
		t.Fatalf("expected default smoke template: %s", rec.Body.String())
	}

	body := strings.NewReader(`{"name":"canvas","namespaces":3,"routesPerNamespace":2,"servicesPerNamespace":2,"replicasPerService":3,"routeToService":"oneToOne"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", body)
	rec = httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"saved":"canvas"`) {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"namespaces":3`) {
		t.Fatalf("topology %d %s", rec.Code, rec.Body.String())
	}
}

func TestDryRunExecute(t *testing.T) {
	s := New("dev", t.TempDir(), "", "", nil, nil)
	body := strings.NewReader(`{"template":"smoke","dryRun":true,"confirm":false,"skipBaseline":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", body)
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != 202 {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	// Wait for async dry-run worker so TempDir cleanup isn't racing persist writes.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m := s.execMgr()
		m.mu.Lock()
		cur := m.cur
		m.mu.Unlock()
		if cur != nil && cur.Status != "running" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestEventsSSE(t *testing.T) {
	s := New("dev", t.TempDir(), "", "", nil, nil)
	s.eventBus().Publish("log", "TEST3", "smoke", logLine{Level: "info", Phase: "CLEANUP", Message: "hello-sse"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?after=0", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.Mux.ServeHTTP(rec, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body := rec.Body.String()
		if strings.Contains(body, "hello-sse") && strings.Contains(body, "event: log") {
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	t.Fatalf("SSE body missing log frame: %q", rec.Body.String())
}

func TestDeleteLoginCommandCluster(t *testing.T) {
	s := New("dev", t.TempDir(), "", "", nil, nil)
	if _, _, err := s.writeTokenKubeconfig(&parsedLogin{
		Name: "lab-target", Server: "https://api.example:6443", Token: "tok",
	}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range s.listClusters() {
		if c.Name == "lab-target" && c.Source == "login-command" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected login-command cluster in list")
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cluster", strings.NewReader(`{"name":"lab-target"}`))
	rec := httptest.NewRecorder()
	s.Mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range s.listClusters() {
		if c.Name == "lab-target" {
			t.Fatal("cluster still listed after delete")
		}
	}
}
