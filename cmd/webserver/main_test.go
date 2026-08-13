package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlers(t *testing.T) {
	srv := httptest.NewServer(newMux("kb-test-pod"))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.TrimSpace(string(body)) != "kb-test-pod" {
		t.Fatalf("body %q", body)
	}

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err = http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s %d", path, resp.StatusCode)
		}
	}
}
