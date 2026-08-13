package main

import (
	"fmt"
	"net/http"
	"os"
)

func podName() string {
	if v := os.Getenv("POD_NAME"); v != "" {
		return v
	}
	if v := os.Getenv("HOSTNAME"); v != "" {
		return v
	}
	return "unknown"
}

func newMux(name string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, name)
	})
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}
	mux.HandleFunc("/healthz", ok)
	mux.HandleFunc("/readyz", ok)
	return mux
}

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if err := http.ListenAndServe(addr, newMux(podName())); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
