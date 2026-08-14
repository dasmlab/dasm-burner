package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) currentExecPath() string {
	return filepath.Join(s.RunDir, "current-exec.json")
}

func (s *Server) writeCurrentExec(run *execRun) {
	if run == nil || s.RunDir == "" {
		return
	}
	snap := run.snapshotTail(40)
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(s.RunDir, 0o755)
	tmp := s.currentExecPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.currentExecPath())
}

func (s *Server) clearCurrentExecFile() {
	_ = os.Remove(s.currentExecPath())
}

func (s *Server) loadCurrentExec() *execRun {
	b, err := os.ReadFile(s.currentExecPath())
	if err != nil {
		return nil
	}
	var run execRun
	if err := json.Unmarshal(b, &run); err != nil {
		return nil
	}
	if run.Status == "running" {
		run.Status = "interrupted"
		now := time.Now()
		run.Finished = &now
		run.Error = "process restarted while run was in progress — UI state restored from disk; apply may still be live on the cluster"
		run.Logs = append(run.Logs, logLine{
			At: now, Level: "warn", Phase: "RESUME",
			Message: "server restarted — restored execution canvas from /data/current-exec.json (worker did not resume)",
		})
		// Mark running pipeline steps as interrupted visually
		for i := range run.Steps {
			if run.Steps[i].Status == stepRunning {
				run.Steps[i].Status = stepFailed
				run.Steps[i].Message = "interrupted by server restart"
				run.Steps[i].Finished = &now
			}
		}
	}
	run.cancel = nil
	run.attachPersist(s)
	s.writeCurrentExec(&run) // persist interrupted marker
	return &run
}

func (r *execRun) attachPersist(s *Server) {
	var last time.Time
	r.publishLog = func(line logLine) {
		seq := s.eventBus().Publish("log", r.Cluster, r.Template, line)
		r.mu.Lock()
		r.LogSeq = seq
		r.mu.Unlock()
	}
	r.onChange = func() {
		now := time.Now()
		if !last.IsZero() && now.Sub(last) < 8*time.Second {
			return
		}
		last = now
		s.writeCurrentExec(r)
		s.publishRunMeta(r)
	}
}

func (s *Server) execMgr() *execManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exec == nil {
		s.exec = &execManager{}
		if run := s.loadCurrentExec(); run != nil {
			s.exec.cur = run
		}
	}
	return s.exec
}
