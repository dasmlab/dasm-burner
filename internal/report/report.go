package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dasmlab/dasm-burner/internal/burner"
	"github.com/dasmlab/dasm-burner/internal/kube"
	"github.com/dasmlab/dasm-burner/internal/runner"
)

// Document is the Phase 4 product: apply + kube-burner collected + OVN/node health.
// When Immutable is true, Health/Open/Close were frozen at end of run and must not
// be refreshed from the live cluster (cleanup would otherwise rewrite history).
type Document struct {
	GeneratedAt     time.Time          `json:"generatedAt"`
	SnapshotID      string             `json:"snapshotId,omitempty"`
	Immutable       bool               `json:"immutable,omitempty"`
	RunID           string             `json:"runId,omitempty"`
	Prefix          string             `json:"prefix,omitempty"`
	Template        string             `json:"template,omitempty"`
	Cluster         string             `json:"cluster,omitempty"`
	Status          string             `json:"status,omitempty"`
	DryRun          bool               `json:"dryRun,omitempty"`
	Started         time.Time          `json:"started,omitempty"`
	Finished        time.Time          `json:"finished,omitempty"`
	Duration        string             `json:"duration,omitempty"`   // wall-clock human
	DurationMs      int64              `json:"durationMs,omitempty"` // wall-clock ms
	Mode            string             `json:"mode,omitempty"`
	BatchCount      int                `json:"batchCount,omitempty"`
	ApplyDuration   string             `json:"applyDuration,omitempty"`
	ApplyDurationMs int64              `json:"applyDurationMs,omitempty"`
	Desired         DesiredCounts      `json:"desired,omitempty"`
	Open            SummaryBox         `json:"open,omitempty"`
	Close           SummaryBox         `json:"close,omitempty"`
	Health          kube.Health        `json:"health"`
	Apply           *runner.Report     `json:"apply,omitempty"`
	JobSummary      map[string]any     `json:"jobSummary,omitempty"`
	Metrics         map[string]Summary `json:"metrics,omitempty"`
	Alerts          []map[string]any   `json:"alerts,omitempty"`
	MetricsArchive  string             `json:"metricsArchive,omitempty"`
	Logs            []RunLogLine       `json:"logs,omitempty"`
	Narrative       string             `json:"narrative"`
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
		tarball := filepath.Join(collectedDir, burner.MetricsTarballName)
		if st, err := os.Stat(tarball); err == nil && !st.IsDir() {
			doc.MetricsArchive = burner.MetricsTarballName
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
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "alert") {
			var alerts []map[string]any
			if json.Unmarshal(b, &alerts) == nil {
				doc.Alerts = append(doc.Alerts, alerts...)
			}
			continue
		}
		if strings.Contains(lower, "jobsummary") {
			attachJobSummary(b, doc)
			continue
		}
		if s, ok := summarizeMetric(name, b); ok {
			doc.Metrics[s.Metric] = s
		}
	}
	return nil
}

func attachJobSummary(b []byte, doc *Document) {
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err == nil && len(rows) > 0 {
		doc.JobSummary = rows[len(rows)-1]
		return
	}
	var one map[string]any
	if err := json.Unmarshal(b, &one); err == nil && one != nil {
		doc.JobSummary = one
	}
}

func summarizeMetric(name string, b []byte) (Summary, bool) {
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) == 0 {
		return Summary{}, false
	}
	// Skip pure jobSummary objects if misnamed file
	if mn, _ := rows[0]["metricName"].(string); strings.EqualFold(mn, "jobSummary") {
		return Summary{}, false
	}
	s := Summary{Metric: name, Count: len(rows)}
	var sum float64
	var n int
	for _, r := range rows {
		v, ok := asFloat(r["value"])
		if !ok {
			continue
		}
		n++
		sum += v
		if n == 1 || v > s.Max {
			s.Max = v
		}
		s.Last = v
		if ts, ok := r["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				s.At = t
			} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
				s.At = t
			}
		}
	}
	if n == 0 {
		return Summary{}, false
	}
	s.Avg = sum / float64(n)
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
		b.WriteString(fmt.Sprintf("Template `%s` · cluster `%s` · status `%s`.\n",
			orDash(doc.Template), orDash(doc.Cluster), orDash(doc.Status)))
	}
	if !doc.Started.IsZero() || !doc.Finished.IsZero() || doc.Duration != "" {
		b.WriteString("\n## Timing\n\n")
		if !doc.Started.IsZero() {
			b.WriteString(fmt.Sprintf("- started: %s\n", doc.Started.Format(time.RFC3339)))
		}
		if !doc.Finished.IsZero() {
			b.WriteString(fmt.Sprintf("- finished: %s\n", doc.Finished.Format(time.RFC3339)))
		}
		if doc.Duration != "" {
			b.WriteString(fmt.Sprintf("- wall duration: %s (%d ms)\n", doc.Duration, doc.DurationMs))
		}
		if doc.ApplyDuration != "" {
			b.WriteString(fmt.Sprintf("- apply duration: %s (%d ms)\n", doc.ApplyDuration, doc.ApplyDurationMs))
		}
		if doc.BatchCount > 0 {
			b.WriteString(fmt.Sprintf("- batches: %d · mode: %s\n", doc.BatchCount, orDash(doc.Mode)))
		}
		b.WriteString("\n")
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
	b.WriteString(fmt.Sprintf("## Cluster health (frozen)\n\nNodes Ready %d / not Ready %d. OVN pods Ready %d/%d (restarts lifetime %d · Δ during run %d). Managed pods Ready %d/%d. OOMKilled %d. Warning events %d.\n\n",
		h.NodesReady, h.NodesNotReady, h.OVNReady, h.OVNPods, h.OVNRestarts, h.OVNRestartsDelta, h.ManagedReady, h.ManagedPods, h.OOMKilled, h.WarningEvents))
	if len(h.OVNDetail) > 0 {
		b.WriteString("### OVN pods by node\n\n")
		for _, p := range h.OVNDetail {
			b.WriteString(fmt.Sprintf("- `%s` node=`%s` ready=%v restarts=%d Δ=%d phase=%s\n",
				p.Name, orDash(p.Node), p.Ready, p.Restarts, p.RestartsDelta, orDash(p.Phase)))
		}
		b.WriteString("\n")
	}
	if doc.JobSummary != nil {
		b.WriteString("## Job summary (kube-burner)\n\n")
		if v, ok := doc.JobSummary["passed"]; ok {
			b.WriteString(fmt.Sprintf("- passed: %v\n", v))
		}
		if v, ok := doc.JobSummary["elapsedTime"]; ok {
			b.WriteString(fmt.Sprintf("- elapsedTime: %v\n", v))
		}
		if v, ok := doc.JobSummary["executionErrors"]; ok && fmt.Sprint(v) != "" {
			b.WriteString(fmt.Sprintf("- executionErrors: %v\n", v))
		}
		b.WriteString("\n")
	}
	if doc.Apply != nil {
		b.WriteString(fmt.Sprintf("Apply mode %s duration %s convergence %.1f%% aborted=%v.\n",
			doc.Apply.Mode, doc.Apply.Duration.Round(time.Second), doc.Apply.Convergence.Overall, doc.Apply.Aborted))
		if doc.Apply.AbortReason != "" {
			b.WriteString("Abort: " + doc.Apply.AbortReason + "\n")
		}
		b.WriteString("\n")
	}
	if len(doc.Logs) > 0 {
		b.WriteString(fmt.Sprintf("## Execute log\n\n%d line(s) frozen from the live canvas.\n\n", len(doc.Logs)))
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
			b.WriteString(fmt.Sprintf("- %s: last %s · max %s · avg %s (n=%d)\n",
				HumanLabel(m.Metric), HumanValue(m.Metric, m.Last), HumanValue(m.Metric, m.Max), HumanValue(m.Metric, m.Avg), m.Count))
		}
		b.WriteString("\n")
	}
	if doc.MetricsArchive != "" {
		b.WriteString(fmt.Sprintf("Metrics archive: `%s`\n\n", doc.MetricsArchive))
	}
	if len(doc.Alerts) > 0 {
		b.WriteString(fmt.Sprintf("## Alerts\n\n%d alert document(s) in collected metrics. Warnings do not fail apply.\n", len(doc.Alerts)))
	}
	return b.String()
}

// CopyMetricsIntoSnapshot copies collected JSON + tarball into reports/<id>/metrics/.
func CopyMetricsIntoSnapshot(collectedDir, snapshotDir string) error {
	if collectedDir == "" {
		return nil
	}
	srcEnts, err := os.ReadDir(collectedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dst := filepath.Join(snapshotDir, "metrics")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range srcEnts {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") && name != burner.MetricsTarballName {
			continue
		}
		if err := copyFile(filepath.Join(collectedDir, name), filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
