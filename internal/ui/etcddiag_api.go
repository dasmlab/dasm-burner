package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/etcddiag"
	"github.com/dasmlab/dasm-burner/internal/kube"
)

type etcdJob struct {
	kind    string
	batchID int
	log     *execRun
}

func (s *Server) ensureEtcdQueue() chan etcdJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.etcdQ == nil {
		s.etcdQ = make(chan etcdJob, 4)
		go s.loopEtcdWorker()
	}
	return s.etcdQ
}

func (s *Server) enqueueEtcd(kind string, batchID int, log *execRun) bool {
	q := s.ensureEtcdQueue()
	select {
	case q <- etcdJob{kind: kind, batchID: batchID, log: log}:
		return true
	default:
		return false
	}
}

func (s *Server) loopEtcdWorker() {
	for job := range s.etcdQ {
		s.performEtcdJob(job)
	}
}

func (s *Server) performEtcdJob(job etcdJob) {
	s.etcdMu.Lock()
	defer s.etcdMu.Unlock()

	cluster := s.currentCluster().Name
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cl, err := s.liveClient(8, 16)
	if err != nil {
		s.eventBus().Publish("etcd", cluster, "", map[string]any{"error": err.Error(), "kind": job.kind})
		if job.log != nil {
			job.log.appendLog("warn", "ETCDDIAG", job.batchID, "queued sample failed: "+err.Error())
		}
		return
	}
	live, ok := cl.(*kube.Live)
	if !ok || live.Clientset() == nil {
		err := fmt.Errorf("etcddiag requires a live cluster clientset")
		s.eventBus().Publish("etcd", cluster, "", map[string]any{"error": err.Error(), "kind": job.kind})
		return
	}
	runID := ""
	m := s.execMgr()
	m.mu.Lock()
	if m.cur != nil {
		runID = m.cur.RunID
	}
	m.mu.Unlock()

	snap, err := etcddiag.SampleLive(ctx, live.Clientset(), runID, cluster, job.batchID)
	if err != nil {
		s.eventBus().Publish("etcd", cluster, "", map[string]any{"error": err.Error(), "kind": job.kind})
		if job.log != nil {
			job.log.appendLog("warn", "ETCDDIAG", job.batchID, err.Error())
		}
		return
	}
	if job.kind == "baseline" {
		snap.Kind = "baseline"
		snap.BaselineAt = snap.GeneratedAt
	}
	id, _ := etcddiag.WriteSnapshot(s.RunDir, snap)
	s.eventBus().Publish("etcd", cluster, "", map[string]any{
		"kind":         job.kind,
		"snapshotId":   id,
		"overallState": snap.OverallState,
		"findingCount": snap.FindingCount,
		"masters":      fmt.Sprintf("%d/%d", snap.MastersReady, snap.MastersTotal),
		"etcd":         fmt.Sprintf("%d/%d", snap.EtcdReady, snap.EtcdTotal),
	})
	if job.log != nil {
		job.log.appendLog("info", "ETCDDIAG", job.batchID,
			fmt.Sprintf("%s · %s · masters=%d/%d etcd=%d/%d findings=%d · snapshot %s",
				job.kind, snap.OverallState, snap.MastersReady, snap.MastersTotal, snap.EtcdReady, snap.EtcdTotal, snap.FindingCount, id))
	}
}

func (s *Server) etcddiagAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.etcddiagGet(w, r)
	case http.MethodPost:
		s.etcddiagSample(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) etcddiagGet(w http.ResponseWriter, r *http.Request) {
	sums, _ := etcddiag.ListSummaries(s.RunDir, 50)
	if snap, err := etcddiag.LoadLatest(s.RunDir); err == nil && snap != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshot": snap, "cached": true, "samples": sums, "rules": etcddiag.RuleCatalog,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": nil, "cached": true, "samples": sums, "rules": etcddiag.RuleCatalog,
	})
}

func (s *Server) etcddiagSample(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BatchID int `json:"batchId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if !s.enqueueEtcd("sample", body.BatchID, s.ensureLogSink()) {
		writeError(w, http.StatusConflict, fmt.Errorf("ETCD worker busy — wait for the in-flight sample"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true, "async": true, "kind": "sample", "stream": eventsPath,
		"message": "sample queued on ETCD worker; Reload latest or watch SSE event etcd",
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) etcddiagBaselineAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.enqueueEtcd("baseline", 0, s.ensureLogSink()) {
		writeError(w, http.StatusConflict, fmt.Errorf("ETCD worker busy — wait for the in-flight sample"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted": true, "async": true, "kind": "baseline", "stream": eventsPath,
		"message": "baseline queued on ETCD worker; Reload latest or watch SSE event etcd",
		"warning": "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

func (s *Server) etcddiagHistoryAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/etcddiag/history")
	path = strings.Trim(path, "/")
	if path != "" {
		snap, err := etcddiag.LoadByID(s.RunDir, path)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("snapshot %s: %w", path, err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"snapshot": snap, "rules": etcddiag.RuleCatalog})
		return
	}
	sums, err := etcddiag.ListSummaries(s.RunDir, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": sums, "rules": etcddiag.RuleCatalog,
		"store": "PVC /data (runDir/etcddiag/<id>/snapshot.json)",
	})
}
