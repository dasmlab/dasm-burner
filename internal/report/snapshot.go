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

// DesiredCounts is the planned topology size frozen into a snapshot.
type DesiredCounts struct {
	Namespaces  int `json:"namespaces"`
	Services    int `json:"services"`
	Routes      int `json:"routes"`
	Deployments int `json:"deployments"`
	Pods        int `json:"pods"`
}

// SummaryBox is an accordion section for Report UI (open = start, close = end).
type SummaryBox struct {
	Title       string            `json:"title"`
	At          time.Time         `json:"at,omitempty"`
	Headline    string            `json:"headline"`
	Highlights  []string          `json:"highlights,omitempty"`
	Health      *kube.Health      `json:"health,omitempty"`
	Convergence *kube.Convergence `json:"convergence,omitempty"`
	Desired     *DesiredCounts    `json:"desired,omitempty"`
}

// Meta is provenance attached when freezing an immutable run snapshot.
type Meta struct {
	Template string
	Cluster  string
	Prefix   string
	Status   string
	DryRun   bool
	Started  time.Time
	Finished time.Time
	Desired  DesiredCounts
	Open     kube.Health // sampled before apply batches (may be zero)
	Logs     []RunLogLine
}

// RunLogLine is one Execute canvas line frozen into the snapshot.
type RunLogLine struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Phase   string    `json:"phase,omitempty"`
	Batch   int       `json:"batch,omitempty"`
	Message string    `json:"message"`
}

// SnapshotID builds a stable id: {runId}-{finishedUnix}.
func SnapshotID(runID string, finished time.Time) string {
	if finished.IsZero() {
		finished = time.Now()
	}
	rid := runID
	if rid == "" {
		rid = "unknown"
	}
	return fmt.Sprintf("%s-%d", rid, finished.Unix())
}

// Freeze turns apply + collected + open/close health into an immutable Document.
// Health for the document body prefers apply.Health (end-of-run), not a live re-query.
func Freeze(apply *runner.Report, collectedDir string, meta Meta) (*Document, error) {
	var health kube.Health
	if apply != nil {
		health = apply.Health
	}
	doc, err := Build(health, apply, collectedDir)
	if err != nil {
		return nil, err
	}
	fin := meta.Finished
	if fin.IsZero() {
		fin = time.Now()
	}
	started := meta.Started
	if started.IsZero() && apply != nil {
		started = apply.Started
	}
	doc.Immutable = true
	doc.GeneratedAt = fin
	doc.Template = meta.Template
	doc.Cluster = meta.Cluster
	doc.Prefix = meta.Prefix
	doc.Status = meta.Status
	doc.DryRun = meta.DryRun
	doc.Started = started
	doc.Finished = fin
	doc.Desired = meta.Desired
	doc.Logs = meta.Logs
	fillRunTiming(doc, apply)
	if doc.RunID == "" && apply != nil {
		doc.RunID = apply.RunID
	}
	doc.SnapshotID = SnapshotID(doc.RunID, fin)

	openAt := meta.Open.SampledAt
	if openAt.IsZero() {
		openAt = started
	}
	des := meta.Desired
	doc.Open = SummaryBox{
		Title:   "Open",
		At:      openAt,
		Desired: &des,
		Headline: fmt.Sprintf("planned %d NS · %d pods · prefix %s",
			des.Namespaces, des.Pods, meta.Prefix),
		Highlights: openHighlights(meta, des),
	}
	if !meta.Open.SampledAt.IsZero() || meta.Open.NodesReady > 0 || meta.Open.OVNPods > 0 {
		h := meta.Open
		doc.Open.Health = &h
		doc.Open.Highlights = append(doc.Open.Highlights,
			fmt.Sprintf("cluster at open: nodes Ready %d / not Ready %d · OVN %d/%d (restarts watermark %d)",
				h.NodesReady, h.NodesNotReady, h.OVNReady, h.OVNPods, h.OVNRestarts))
	}

	closeAt := fin
	if apply != nil && !apply.Health.SampledAt.IsZero() {
		closeAt = apply.Health.SampledAt
	}
	doc.Close = SummaryBox{
		Title:    "Close",
		At:       closeAt,
		Headline: closeHeadline(apply, meta.Status),
		Desired:  &des,
	}
	if apply != nil {
		h := kube.ApplyOVNRestartDeltas(meta.Open, apply.Health)
		doc.Health = h
		apply.Health = h
		c := apply.Convergence
		doc.Close.Health = &h
		doc.Close.Convergence = &c
		doc.Close.Highlights = closeHighlights(apply)
	}

	doc.Narrative = narrative(doc)
	return doc, nil
}

// fillRunTiming sets wall-clock Duration / DurationMs and apply-mode helpers.
func fillRunTiming(doc *Document, apply *runner.Report) {
	if doc == nil {
		return
	}
	started, fin := doc.Started, doc.Finished
	if started.IsZero() && apply != nil && !apply.Started.IsZero() {
		started = apply.Started
		doc.Started = started
	}
	if fin.IsZero() && apply != nil && !apply.Finished.IsZero() {
		fin = apply.Finished
		doc.Finished = fin
	}
	if !started.IsZero() && !fin.IsZero() && !fin.Before(started) {
		d := fin.Sub(started)
		doc.DurationMs = d.Milliseconds()
		doc.Duration = d.Round(time.Second).String()
	} else if apply != nil && apply.Duration > 0 {
		doc.DurationMs = apply.Duration.Milliseconds()
		doc.Duration = apply.Duration.Round(time.Second).String()
	}
	if apply != nil {
		doc.Mode = apply.Mode
		doc.BatchCount = len(apply.Batches)
		doc.ApplyDuration = apply.Duration.Round(time.Second).String()
		doc.ApplyDurationMs = apply.Duration.Milliseconds()
	}
}

func openHighlights(meta Meta, des DesiredCounts) []string {
	out := []string{
		fmt.Sprintf("template %s", orDash(meta.Template)),
		fmt.Sprintf("cluster %s", orDash(meta.Cluster)),
		fmt.Sprintf("desired objects NS=%d svc=%d rt=%d deploy=%d pods=%d",
			des.Namespaces, des.Services, des.Routes, des.Deployments, des.Pods),
	}
	if meta.DryRun {
		out = append(out, "dry-run (no create)")
	}
	return out
}

func closeHeadline(apply *runner.Report, status string) string {
	if apply == nil {
		return status
	}
	parts := []string{fmt.Sprintf("convergence %.1f%%", apply.Convergence.Overall)}
	if status != "" {
		parts = append([]string{status}, parts...)
	}
	if apply.Aborted {
		parts = append(parts, "aborted")
	}
	h := apply.Health
	if h.ManagedPods > 0 || h.ManagedReady > 0 {
		parts = append(parts, fmt.Sprintf("managed Ready %d/%d", h.ManagedReady, h.ManagedPods))
	}
	return strings.Join(parts, " · ")
}

func closeHighlights(apply *runner.Report) []string {
	h := apply.Health
	out := []string{
		fmt.Sprintf("nodes Ready %d / not Ready %d", h.NodesReady, h.NodesNotReady),
		fmt.Sprintf("OVN Ready %d/%d", h.OVNReady, h.OVNPods),
		fmt.Sprintf("OVN restarts Δ during run %d (lifetime %d)", h.OVNRestartsDelta, h.OVNRestarts),
		fmt.Sprintf("managed Ready %d/%d", h.ManagedReady, h.ManagedPods),
		fmt.Sprintf("OOM %d", h.OOMKilled),
		fmt.Sprintf("warn events %d", h.WarningEvents),
		fmt.Sprintf("mode %s", apply.Mode),
		fmt.Sprintf("apply duration %s", apply.Duration.Round(time.Second)),
		fmt.Sprintf("batches %d", len(apply.Batches)),
	}
	for _, p := range h.OVNDetail {
		if p.RestartsDelta > 0 || !p.Ready {
			out = append(out, fmt.Sprintf("OVN %s @%s ready=%v Δrestarts=%d", p.Name, orDash(p.Node), p.Ready, p.RestartsDelta))
		}
	}
	if apply.AbortReason != "" {
		out = append(out, "abort: "+apply.AbortReason)
	}
	for _, e := range apply.Errors {
		if e != "" {
			out = append(out, "error: "+e)
		}
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// WriteSnapshot writes an immutable archive under {runDir}/reports/{snapshotId}/
// and refreshes {runDir}/report.json as a pointer to the latest snapshot.
func WriteSnapshot(runDir string, doc *Document, apply *runner.Report) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("document is nil")
	}
	if doc.SnapshotID == "" {
		doc.SnapshotID = SnapshotID(doc.RunID, doc.Finished)
	}
	dir := filepath.Join(runDir, "reports", doc.SnapshotID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), b, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(doc.Narrative+"\n"), 0o644); err != nil {
		return "", err
	}
	if apply != nil {
		ab, err := json.MarshalIndent(apply, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, "apply-report.json"), ab, 0o644); err != nil {
			return "", err
		}
	}
	collected := filepath.Join(runDir, "kube-burner", "collected")
	_ = CopyMetricsIntoSnapshot(collected, dir)
	_ = writeSummaryFile(dir, listItemFromDoc(doc))
	// Latest pointer for legacy GET /api/v1/report
	_ = Write(runDir, doc)
	return doc.SnapshotID, nil
}

func listItemFromDoc(doc *Document) ListItem {
	if doc == nil {
		return ListItem{}
	}
	item := ListItem{
		SnapshotID:    doc.SnapshotID,
		RunID:         doc.RunID,
		Prefix:        doc.Prefix,
		Template:      doc.Template,
		Cluster:       doc.Cluster,
		Status:        doc.Status,
		DryRun:        doc.DryRun,
		Immutable:     doc.Immutable,
		GeneratedAt:   doc.GeneratedAt,
		Started:       doc.Started,
		Finished:      doc.Finished,
		Duration:      doc.Duration,
		DurationMs:    doc.DurationMs,
		BatchCount:    doc.BatchCount,
		OpenHeadline:  doc.Open.Headline,
		CloseHeadline: doc.Close.Headline,
	}
	if doc.Apply != nil {
		item.ConvergenceOverall = doc.Apply.Convergence.Overall
	} else if doc.Close.Convergence != nil {
		item.ConvergenceOverall = doc.Close.Convergence.Overall
	}
	return item
}

func writeSummaryFile(snapshotDir string, item ListItem) error {
	if item.SnapshotID == "" {
		return nil
	}
	b, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(snapshotDir, "summary.json"), b, 0o644)
}

// ListItem is a compact row for the reports index.
type ListItem struct {
	SnapshotID         string    `json:"snapshotId"`
	RunID              string    `json:"runId"`
	Prefix             string    `json:"prefix,omitempty"`
	Template           string    `json:"template,omitempty"`
	Cluster            string    `json:"cluster,omitempty"`
	Status             string    `json:"status,omitempty"`
	DryRun             bool      `json:"dryRun,omitempty"`
	Immutable          bool      `json:"immutable"`
	GeneratedAt        time.Time `json:"generatedAt"`
	Started            time.Time `json:"started,omitempty"`
	Finished           time.Time `json:"finished,omitempty"`
	Duration           string    `json:"duration,omitempty"`
	DurationMs         int64     `json:"durationMs,omitempty"`
	BatchCount         int       `json:"batchCount,omitempty"`
	ConvergenceOverall float64   `json:"convergenceOverall,omitempty"`
	OpenHeadline       string    `json:"openHeadline,omitempty"`
	CloseHeadline      string    `json:"closeHeadline,omitempty"`
}

// ListSnapshots scans {runDir}/reports/*/snapshot.json (newest first).
func ListSnapshots(runDir string) ([]ListItem, error) {
	root := filepath.Join(runDir, "reports")
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ListItem
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		item, ok := loadSummary(runDir, e.Name())
		if !ok {
			doc, err := LoadSnapshot(runDir, e.Name())
			if err != nil || doc == nil {
				continue
			}
			item = listItemFromDoc(doc)
			_ = writeSummaryFile(filepath.Join(root, e.Name()), item)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GeneratedAt.After(out[j].GeneratedAt)
	})
	return out, nil
}

func loadSummary(runDir, snapshotID string) (ListItem, bool) {
	path := filepath.Join(runDir, "reports", snapshotID, "summary.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return ListItem{}, false
	}
	var item ListItem
	if err := json.Unmarshal(b, &item); err != nil || item.SnapshotID == "" {
		return ListItem{}, false
	}
	return item, true
}

// LoadSnapshot reads an immutable snapshot by id.
func LoadSnapshot(runDir, snapshotID string) (*Document, error) {
	snapshotID = filepath.Base(strings.TrimSpace(snapshotID))
	if snapshotID == "" || snapshotID == "." || snapshotID == ".." {
		return nil, fmt.Errorf("invalid snapshot id")
	}
	path := filepath.Join(runDir, "reports", snapshotID, "snapshot.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if doc.SnapshotID == "" {
		doc.SnapshotID = snapshotID
	}
	return &doc, nil
}

// LatestSnapshot returns the newest immutable snapshot, if any.
func LatestSnapshot(runDir string) (*Document, error) {
	list, err := ListSnapshots(runDir)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, os.ErrNotExist
	}
	return LoadSnapshot(runDir, list[0].SnapshotID)
}
