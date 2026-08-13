package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
)

// Document is the Phase 4 product: apply + kube-burner collected + OVN/node health.
// When Immutable is true, Health/Open/Close were frozen at end of run and must not
// be refreshed from the live cluster (cleanup would otherwise rewrite history).
type Document struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	SnapshotID  string             `json:"snapshotId,omitempty"`
	Immutable   bool               `json:"immutable,omitempty"`
	RunID       string             `json:"runId,omitempty"`
	Prefix      string             `json:"prefix,omitempty"`
	Template    string             `json:"template,omitempty"`
	Cluster     string             `json:"cluster,omitempty"`
	Status      string             `json:"status,omitempty"`
	DryRun      bool               `json:"dryRun,omitempty"`
	Started     time.Time          `json:"started,omitempty"`
	Finished    time.Time          `json:"finished,omitempty"`
	Desired     DesiredCounts      `json:"desired,omitempty"`
	Open        SummaryBox         `json:"open,omitempty"`
	Close       SummaryBox         `json:"close,omitempty"`
	Health      kube.Health        `json:"health"`
	Apply       *runner.Report     `json:"apply,omitempty"`
	Metrics     map[string]Summary `json:"metrics,omitempty"`
	Alerts      []map[string]any   `json:"alerts,omitempty"`
	Narrative   string             `json:"narrative"`
}

type Summary struct {
	Metric string    `json:"metric"`
	Count  int       `json:"count"`
	Last   float64   `json:"last"`
	Max    float64   `json:"max"`
	Avg    float64   `json:"avg"`
	At     time.Time `json:"at,omitempty"`
}

func Build(health kube.Health, apply *runner.Report, collectedDir string) (*Document, error) {
	doc := &Document{
		GeneratedAt: time.Now(),
		Health:      health,
		Apply:       apply,
		Metrics:     map[string]Summary{},
	}
	if apply != nil {
		doc.RunID = apply.RunID
	}
	if collectedDir != "" {
		if err := loadCollected(collectedDir, doc); err != nil {
			return nil, err
		}
	}
	doc.Narrative = narrative(doc)
	return doc, nil
}

func Write(dir string, doc *Document) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), b, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(doc.Narrative+"\n"), 0o644)
}

func loadCollected(dir string, doc *Document) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if strings.HasPrefix(name, "alert") {
			var alerts []map[string]any
			if json.Unmarshal(b, &alerts) == nil {
				doc.Alerts = append(doc.Alerts, alerts...)
			}
			continue
		}
		if s, ok := summarizeMetric(name, b); ok {
			doc.Metrics[s.Metric] = s
		}
	}
	return nil
}

func summarizeMetric(name string, b []byte) (Summary, bool) {
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) == 0 {
		return Summary{}, false
	}
	s := Summary{Metric: name, Count: len(rows)}
	var sum float64
	var n int
	for _, r := range rows {
		v, ok := asFloat(r["value"])
		if !ok {
			v, ok = asFloat(r["query"])
		}
		if !ok {
			continue
		}
		n++
		sum += v
		if n == 1 || v > s.Max {
			s.Max = v
		}
		s.Last = v
	}
	if n > 0 {
		s.Avg = sum / float64(n)
	}
	return s, true
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		var f float64
		_, err := fmt.Sscanf(t, "%f", &f)
		return f, err == nil
	default:
		return 0, false
	}
}

func narrative(doc *Document) string {
	var b strings.Builder
	b.WriteString("# dasm-burner report\n\n")
	b.WriteString(fmt.Sprintf("Generated %s", doc.GeneratedAt.Format(time.RFC3339)))
	if doc.Immutable {
		b.WriteString(" (immutable snapshot)")
	}
	b.WriteString("\n\n")
	if doc.SnapshotID != "" {
		b.WriteString(fmt.Sprintf("Snapshot `%s`.\n", doc.SnapshotID))
	}
	if doc.RunID != "" {
		b.WriteString(fmt.Sprintf("Run `%s`", doc.RunID))
		if doc.Prefix != "" {
			b.WriteString(fmt.Sprintf(" · prefix `%s`", doc.Prefix))
		}
		b.WriteString(".\n")
	}
	if doc.Template != "" || doc.Cluster != "" {
		b.WriteString(fmt.Sprintf("Template `%s` · cluster `%s` · status `%s`.\n\n",
			orDash(doc.Template), orDash(doc.Cluster), orDash(doc.Status)))
	}
	if doc.Open.Headline != "" {
		b.WriteString("## Open\n\n" + doc.Open.Headline + "\n")
		for _, h := range doc.Open.Highlights {
			b.WriteString("- " + h + "\n")
		}
		b.WriteString("\n")
	}
	if doc.Close.Headline != "" {
		b.WriteString("## Close\n\n" + doc.Close.Headline + "\n")
		for _, h := range doc.Close.Highlights {
			b.WriteString("- " + h + "\n")
		}
		b.WriteString("\n")
	}
	h := doc.Health
	b.WriteString(fmt.Sprintf("## Cluster health (frozen)\n\nNodes Ready %d / not Ready %d. OVN pods Ready %d/%d (restarts %d). Managed pods Ready %d/%d. OOMKilled %d. Warning events %d.\n\n",
		h.NodesReady, h.NodesNotReady, h.OVNReady, h.OVNPods, h.OVNRestarts, h.ManagedReady, h.ManagedPods, h.OOMKilled, h.WarningEvents))
	if doc.Apply != nil {
		b.WriteString(fmt.Sprintf("Apply mode %s duration %s convergence %.1f%% aborted=%v.\n",
			doc.Apply.Mode, doc.Apply.Duration, doc.Apply.Convergence.Overall, doc.Apply.Aborted))
		if doc.Apply.AbortReason != "" {
			b.WriteString("Abort: " + doc.Apply.AbortReason + "\n")
		}
		b.WriteString("\n")
	}
	if len(doc.Metrics) > 0 {
		b.WriteString("## Prometheus / kube-burner\n\n")
		keys := make([]string, 0, len(doc.Metrics))
		for k := range doc.Metrics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m := doc.Metrics[k]
			b.WriteString(fmt.Sprintf("- %s: n=%d last=%.4g max=%.4g avg=%.4g\n", m.Metric, m.Count, m.Last, m.Max, m.Avg))
		}
		b.WriteString("\n")
	}
	if len(doc.Alerts) > 0 {
		b.WriteString(fmt.Sprintf("## Alerts\n\n%d alert document(s) in collected metrics. Warnings do not fail apply.\n", len(doc.Alerts)))
	}
	return b.String()
}
