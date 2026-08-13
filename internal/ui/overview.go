package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dasmlab/dasm-burner/internal/auth"
	"github.com/dasmlab/dasm-burner/internal/report"
	"github.com/dasmlab/dasm-burner/internal/runner"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

// healthBaseline is a shared (per dasm-burner instance) restart counter baseline
// keyed by cluster display name. It does not reset real kube restart counters —
// it only defines "since last clear" for the Health UI. All Keycloak users share it.
type healthBaseline struct {
	Cluster     string    `json:"cluster"`
	OVNRestarts int       `json:"ovnRestarts"`
	ClearedAt   time.Time `json:"clearedAt"`
	ClearedBy   string    `json:"clearedBy,omitempty"`
}

type baselineFile struct {
	Clusters map[string]healthBaseline `json:"clusters"`
}

var baselineMu sync.Mutex

func (s *Server) baselinePath() string {
	return filepath.Join(s.RunDir, "health-baselines.json")
}

func (s *Server) loadBaselines() baselineFile {
	var f baselineFile
	b, err := os.ReadFile(s.baselinePath())
	if err != nil {
		return baselineFile{Clusters: map[string]healthBaseline{}}
	}
	_ = json.Unmarshal(b, &f)
	if f.Clusters == nil {
		f.Clusters = map[string]healthBaseline{}
	}
	return f
}

func (s *Server) saveBaselines(f baselineFile) error {
	_ = os.MkdirAll(s.RunDir, 0o755)
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.baselinePath(), b, 0o644)
}

func (s *Server) baselineFor(cluster string) *healthBaseline {
	baselineMu.Lock()
	defer baselineMu.Unlock()
	f := s.loadBaselines()
	b, ok := f.Clusters[cluster]
	if !ok {
		return nil
	}
	cp := b
	return &cp
}

func (s *Server) clearBaseline(cluster string, ovnRestarts int, who string) healthBaseline {
	baselineMu.Lock()
	defer baselineMu.Unlock()
	f := s.loadBaselines()
	if f.Clusters == nil {
		f.Clusters = map[string]healthBaseline{}
	}
	b := healthBaseline{
		Cluster:     cluster,
		OVNRestarts: ovnRestarts,
		ClearedAt:   time.Now(),
		ClearedBy:   who,
	}
	f.Clusters[cluster] = b
	_ = s.saveBaselines(f)
	return b
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cluster := s.currentCluster()
	cfg, err := s.cfg()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	g, err := topology.Generate(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var decision runner.Decision
	var healthOK bool
	if cl, err := s.liveClient(20, 40); err == nil {
		if h, err := cl.ClusterHealth(r.Context(), ""); err == nil {
			decision = runner.Evaluate(cfg.Safety, h)
			healthOK = true
		}
	}

	base := s.baselineFor(cluster.Name)
	restartsSince := 0
	ovnRestarts := decision.Health.OVNRestarts
	if base != nil {
		restartsSince = ovnRestarts - base.OVNRestarts
		if restartsSince < 0 {
			restartsSince = 0
		}
	} else {
		// No clear yet — treat "since clear" as unknown; expose raw total only.
		restartsSince = -1
	}

	intended := s.listTemplates()

	var live []liveRunRow
	var managedTotal int
	if st, err := s.computeCleanupStatus(r, ""); err == nil && st != nil {
		live = st.LiveRuns
		managedTotal = st.ManagedTotal
	}

	// Enrich live rows: already have template from history when known.
	completed := s.completedForCluster(cluster.Name)

	writeJSON(w, http.StatusOK, map[string]any{
		"cluster":            cluster,
		"health":             decision,
		"healthOK":           healthOK,
		"baseline":           base,
		"ovnRestarts":        ovnRestarts,
		"restartsSinceClear": restartsSince,
		"activeTemplate":     s.activeTemplateName(),
		"activePlan": map[string]any{
			"name":   cfg.Metadata.Name,
			"runId":  g.RunID,
			"counts": g.Counts,
		},
		"intended":     intended,
		"live":         live,
		"managedTotal": managedTotal,
		"completed":    completed,
		"note":         "Restart baseline is shared for this dasm-burner instance per cluster (all Keycloak users). It does not reset kube restart counters.",
		"warning":      "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}

type completedMix struct {
	SnapshotID         string    `json:"snapshotId"`
	RunID              string    `json:"runId"`
	Prefix             string    `json:"prefix,omitempty"`
	Template           string    `json:"template,omitempty"`
	Cluster            string    `json:"cluster,omitempty"`
	Status             string    `json:"status,omitempty"`
	Finished           time.Time `json:"finished,omitempty"`
	ConvergenceOverall float64   `json:"convergenceOverall,omitempty"`
	CloseHeadline      string    `json:"closeHeadline,omitempty"`
}

func (s *Server) completedForCluster(cluster string) []completedMix {
	list, err := report.ListSnapshots(s.RunDir)
	if err != nil {
		return nil
	}
	var out []completedMix
	for _, item := range list {
		if item.DryRun {
			continue
		}
		// Cluster-sensitive: match current cluster, or include unscoped legacy snapshots.
		if item.Cluster != "" && cluster != "" && item.Cluster != cluster {
			continue
		}
		out = append(out, completedMix{
			SnapshotID:         item.SnapshotID,
			RunID:              item.RunID,
			Prefix:             item.Prefix,
			Template:           item.Template,
			Cluster:            item.Cluster,
			Status:             item.Status,
			Finished:           item.Finished,
			ConvergenceOverall: item.ConvergenceOverall,
			CloseHeadline:      item.CloseHeadline,
		})
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func (s *Server) healthBaselineAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cluster := s.currentCluster().Name
		writeJSON(w, http.StatusOK, map[string]any{
			"cluster":  cluster,
			"baseline": s.baselineFor(cluster),
			"note":     "Shared per instance + cluster. Clear sets OVN restart watermark to current total.",
		})
	case http.MethodPost:
		s.clearHealthBaseline(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) clearHealthBaseline(w http.ResponseWriter, r *http.Request) {
	cluster := s.currentCluster()
	cfg, err := s.cfg()
	if err != nil {
		// still allow clear with zero if no config
		cfg = nil
	}
	ovn := 0
	if cl, err := s.liveClient(20, 40); err == nil {
		runID := ""
		if cfg != nil {
			if g, err := topology.Generate(cfg); err == nil {
				runID = g.RunID
			}
		}
		if h, err := cl.ClusterHealth(r.Context(), runID); err == nil {
			ovn = h.OVNRestarts
		}
	}
	who := "unknown"
	if u, ok := auth.UserFromContext(r.Context()); ok && u != nil {
		who = u.PreferredUsername
		if who == "" {
			who = u.Name
		}
	}
	b := s.clearBaseline(cluster.Name, ovn, who)
	writeJSON(w, http.StatusOK, map[string]any{
		"baseline": b,
		"message":  fmt.Sprintf("Cleared OVN restart watermark to %d on %s (by %s). Shared across users of this instance.", ovn, cluster.Name, who),
		"note":     "Kube pod restart counters are unchanged — only the Health UI delta resets.",
		"warning":  "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT",
	})
}
