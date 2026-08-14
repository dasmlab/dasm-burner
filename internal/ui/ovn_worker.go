package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/ovndiag"
)

type ovnJob struct {
	kind    string // sample | baseline
	batchID int
	log     *execRun
}

func (s *Server) ensureOVNQueue() chan ovnJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ovnQ == nil {
		s.ovnQ = make(chan ovnJob, 4)
		go s.loopOVNWorker()
	}
	return s.ovnQ
}

func (s *Server) enqueueOVN(kind string, batchID int, log *execRun) bool {
	q := s.ensureOVNQueue()
	job := ovnJob{kind: kind, batchID: batchID, log: log}
	select {
	case q <- job:
		return true
	default:
		return false
	}
}

func (s *Server) loopOVNWorker() {
	for job := range s.ovnQ {
		s.performOVNJob(job)
	}
}

func (s *Server) performOVNJob(job ovnJob) {
	s.ovnMu.Lock()
	defer s.ovnMu.Unlock()

	cluster := s.currentCluster().Name
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cl, err := s.liveClient(8, 16)
	if err != nil {
		s.eventBus().Publish("ovn", cluster, "", map[string]any{"error": err.Error(), "kind": job.kind})
		if job.log != nil {
			job.log.appendLog("warn", "OVNDIAG", job.batchID, "queued sample failed: "+err.Error())
		}
		return
	}
	live, ok := cl.(*kube.Live)
	if !ok || live.Clientset() == nil {
		err := fmt.Errorf("ovndiag requires a live cluster clientset")
		s.eventBus().Publish("ovn", cluster, "", map[string]any{"error": err.Error(), "kind": job.kind})
		return
	}
	runID := ""
	m := s.execMgr()
	m.mu.Lock()
	if m.cur != nil {
		runID = m.cur.RunID
	}
	m.mu.Unlock()

	snap, err := ovndiag.SampleLive(ctx, live.Clientset(), live.Dynamic(), s.ovnBaseline(), runID, cluster, job.batchID)
	if err != nil {
		s.eventBus().Publish("ovn", cluster, "", map[string]any{"error": err.Error(), "kind": job.kind})
		if job.log != nil {
			job.log.appendLog("warn", "OVNDIAG", job.batchID, err.Error())
		}
		return
	}
	if job.kind == "baseline" {
		s.ovnBaseline().Capture(snap.Nodes)
		snap.BaselineAt = s.ovnBaseline().At()
	}
	id, _ := ovndiag.WriteSnapshot(s.RunDir, snap)
	s.eventBus().Publish("ovn", cluster, "", map[string]any{
		"kind":         job.kind,
		"snapshotId":   id,
		"overallState": snap.OverallState,
		"findingCount": len(snap.Findings),
		"nodes":        len(snap.Nodes),
	})
	if job.log != nil {
		job.log.appendLog("info", "OVNDIAG", job.batchID,
			fmt.Sprintf("%s · %s · findings=%d · snapshot %s", job.kind, snap.OverallState, len(snap.Findings), id))
	}
}

func (s *Server) loopStatusPublisher() {
	t := time.NewTicker(12 * time.Second)
	defer t.Stop()
	for range t.C {
		if s.isCleanupBusy() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			st, err := s.computeCleanupStatus(ctx, s.activeTemplateName())
			cancel()
			if err == nil && st != nil {
				s.publishCleanup(st)
			}
		}
	}
}

func (s *Server) publishCleanup(st *cleanupStatusPayload) {
	if st == nil {
		return
	}
	s.eventBus().Publish("cleanup", s.currentCluster().Name, "", map[string]any{
		"template":     st.Template,
		"liveRuns":     st.LiveRuns,
		"managedTotal": st.ManagedTotal,
		"cleaning":     s.isCleanupBusy(),
		"cluster":      s.currentCluster().Name,
	})
}

func (s *Server) publishRunMeta(run *execRun) {
	if run == nil {
		return
	}
	s.eventBus().Publish("run", run.Cluster, run.Template, run.snapshotMeta())
}
