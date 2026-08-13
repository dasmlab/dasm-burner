package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type runHistoryEntry struct {
	RunID      string    `json:"runId"`
	Template   string    `json:"template"`
	Prefix     string    `json:"prefix"`
	Seed       int64     `json:"seed"`
	DryRun     bool      `json:"dryRun"`
	Started    time.Time `json:"started"`
	Finished   time.Time `json:"finished,omitempty"`
	Status     string    `json:"status"`
	Cluster    string    `json:"cluster,omitempty"`
	SnapshotID string    `json:"snapshotId,omitempty"`
}

type runHistory struct {
	Entries []runHistoryEntry `json:"entries"`
}

func (s *Server) historyPath() string {
	return filepath.Join(s.RunDir, "run-history.json")
}

func (s *Server) loadHistory() runHistory {
	var h runHistory
	b, err := os.ReadFile(s.historyPath())
	if err != nil {
		return h
	}
	_ = json.Unmarshal(b, &h)
	return h
}

func (s *Server) saveHistory(h runHistory) error {
	_ = os.MkdirAll(s.RunDir, 0o755)
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.historyPath(), b, 0o644)
}

var historyMu sync.Mutex

func (s *Server) recordHistory(e runHistoryEntry) {
	historyMu.Lock()
	defer historyMu.Unlock()
	h := s.loadHistory()
	// de-dupe same runId
	out := make([]runHistoryEntry, 0, len(h.Entries)+1)
	for _, x := range h.Entries {
		if x.RunID == e.RunID && x.Template == e.Template {
			continue
		}
		out = append(out, x)
	}
	out = append(out, e)
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	h.Entries = out
	_ = s.saveHistory(h)
}

func (s *Server) lastRealRun(template string) *runHistoryEntry {
	h := s.loadHistory()
	for i := len(h.Entries) - 1; i >= 0; i-- {
		e := h.Entries[i]
		if e.DryRun {
			continue
		}
		if template != "" && e.Template != template {
			continue
		}
		cp := e
		return &cp
	}
	return nil
}

func (s *Server) runsForTemplate(template string) []runHistoryEntry {
	h := s.loadHistory()
	var out []runHistoryEntry
	for _, e := range h.Entries {
		if e.DryRun {
			continue
		}
		if template != "" && e.Template != template {
			continue
		}
		out = append(out, e)
	}
	return out
}

func prefixForRun(runID string) string {
	if runID == "" {
		return ""
	}
	return fmt.Sprintf("kb-%s", runID)
}
