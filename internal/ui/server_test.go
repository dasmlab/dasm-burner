package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
}
