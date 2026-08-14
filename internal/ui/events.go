package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) eventBus() *EventBus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bus == nil {
		s.bus = NewEventBus()
	}
	return s.bus
}

func (s *Server) eventsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	after := uint64(0)
	if v := strings.TrimSpace(r.Header.Get("Last-Event-ID")); v != "" {
		after, _ = strconv.ParseUint(v, 10, 64)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("after")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			after = n
		}
	}
	clusterF := strings.TrimSpace(r.URL.Query().Get("cluster"))
	templateF := strings.TrimSpace(r.URL.Query().Get("template"))

	bus := s.eventBus()
	id, ch, replay := bus.Subscribe(after)
	defer bus.Unsubscribe(id)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	writeEv := func(ev BusEvent) bool {
		if clusterF != "" && ev.Cluster != "" && ev.Cluster != clusterF {
			return true
		}
		if templateF != "" && ev.Template != "" && ev.Template != templateF && ev.Kind == "log" {
			phaseOK := false
			var line struct {
				Phase string `json:"phase"`
			}
			if json.Unmarshal(ev.Data, &line) == nil {
				switch line.Phase {
				case "CLEANUP", "CLUSTER", "STATE", "KUBELET", "CANCEL", "FAILED", "MEM":
					phaseOK = true
				}
			}
			if !phaseOK {
				return true
			}
		}
		payload, err := json.Marshal(ev)
		if err != nil {
			return true
		}
		kind := ev.Kind
		if kind == "" {
			kind = "message"
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, kind, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	for _, ev := range replay {
		if !writeEv(ev) {
			return
		}
	}

	ping := time.NewTicker(12 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !writeEv(ev) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
