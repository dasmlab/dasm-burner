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

// cascadeModel is the density failure order we reproduce on TEST3-class clusters.
var cascadeModel = map[string]any{
	"order": []string{"kube-apiserver", "etcd", "ovn-kube-node / masters"},
	"stages": []map[string]string{
		{"id": "idle", "name": "Idle", "see": "Ready 3/3, RSS near baseline"},
		{"id": "api_flex", "name": "API flex", "see": "kube-apiserver RSS climbs; LIST/WATCH cache; etcd still Ready"},
		{"id": "etcd_flex", "name": "etcd flex", "see": "etcd timeouts / member not Ready / MemoryPressure"},
		{"id": "collapse", "name": "Collapse", "see": "master NotReady, OVN timeouts, API flaps"},
		{"id": "leftover", "name": "Leftover RSS", "see": "workload deleted; API RSS still fat until static-pod restart"},
	},
	"lab": "maxPods on workers (this cluster typically 1000) · host prefix /22 so IPs are not the cliff",
	"rss": "Working set (RSS) is the RAM kube-apiserver is sitting on. Watch-cache grows it; DELETE does not give it back to the node. Cascade is live per sample: idle / api_flex / etcd_flex / collapse / leftover.",
}

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

	snap, err := etcddiag.SampleLive(ctx, live.Clientset(), live.Dynamic(), runID, cluster, job.batchID)
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
	} else if base, err := etcddiag.LoadBaseline(s.RunDir); err == nil && base != nil {
		etcddiag.CompareBaseline(snap, base)
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
			fmt.Sprintf("%s · %s · cascade=%s · apiRSS=%.0fMi etcdRSS=%.0fMi ovnRSS=%.0fMi pods=%d · snapshot %s",
				job.kind, snap.OverallState, snap.Cascade, snap.APIRSSMi, snap.EtcdRSSMi, snap.OVNRSSMi, snap.WorkloadPods, id))
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
			"snapshot": snap, "cached": true, "samples": sums, "series": etcddiag.LoadSeries(s.RunDir),
			"rules": etcddiag.RuleCatalog, "model": cascadeModel,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": nil, "cached": true, "samples": sums, "series": etcddiag.LoadSeries(s.RunDir),
		"rules": etcddiag.RuleCatalog, "model": cascadeModel,
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
		writeJSON(w, http.StatusOK, map[string]any{"snapshot": snap, "rules": etcddiag.RuleCatalog, "model": cascadeModel})
		return
	}
	sums, err := etcddiag.ListSummaries(s.RunDir, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": sums, "series": etcddiag.LoadSeries(s.RunDir), "rules": etcddiag.RuleCatalog,
		"model": cascadeModel,
		"store": "PVC /data (runDir/etcddiag/<id>/snapshot.json + series.json)",
	})
}
