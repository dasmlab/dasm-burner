package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CleanupLogLine is one line captured during a cleanup job.
type CleanupLogLine struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// CleanupObjectTotals are managed-object counts sampled before delete (cascade via NS).
type CleanupObjectTotals struct {
	Namespaces  int `json:"namespaces"`
	Services    int `json:"services"`
	Routes      int `json:"routes"`
	Deployments int `json:"deployments"`
	Pods        int `json:"pods"`
}

// CleanupReport is an immutable record of one cleanup job.
type CleanupReport struct {
	ID                 string              `json:"id"`
	Scope              string              `json:"scope"` // last | template | all
	Template           string              `json:"template,omitempty"`
	Cluster            string              `json:"cluster,omitempty"`
	DryRun             bool                `json:"dryRun"`
	Waited             bool                `json:"waited"`
	Status             string              `json:"status"` // passed | failed | partial
	RunIDs             []string            `json:"runIds,omitempty"`
	Started            time.Time           `json:"started"`
	Finished           time.Time           `json:"finished"`
	DurationMs         int64               `json:"durationMs"`
	Duration           string              `json:"duration"` // human, e.g. 12m34s
	Targeted           CleanupObjectTotals `json:"targeted"`
	DeletedNS          int                 `json:"deletedNamespaces"`
	Remaining          int                 `json:"remainingNamespaces"`
	Namespaces         []string            `json:"namespaces,omitempty"`
	Error              string              `json:"error,omitempty"`
	Logs               []CleanupLogLine    `json:"logs,omitempty"`
	ClusterObservation *ClusterObservation `json:"clusterObservation,omitempty"`
	Warning            string              `json:"warning"`
}

// ClusterObservation captures node/monitoring health during cleanup.
type ClusterObservation struct {
	Samples           []ClusterSample   `json:"samples,omitempty"`
	Incidents         []ClusterIncident `json:"incidents,omitempty"`
	Summary           string            `json:"summary,omitempty"`
	MaxNotReady       int               `json:"maxNotReady"`
	MaxNotReadyDurSec int64             `json:"maxNotReadyDurationSec,omitempty"`
	MonitoringOOM     int               `json:"monitoringOOMTotal"`
	WorstNodes        []string          `json:"worstNodes,omitempty"`
}

// ClusterSample is one cleanup-watch poll.
type ClusterSample struct {
	At                 time.Time `json:"at"`
	NodesReady         int       `json:"nodesReady"`
	NodesNotReady      int       `json:"nodesNotReady"`
	MemoryPressure     int       `json:"memoryPressure"`
	DiskPressure       int       `json:"diskPressure"`
	PIDPressure        int       `json:"pidPressure"`
	MonitoringReady    int       `json:"monitoringReady"`
	MonitoringTotal    int       `json:"monitoringTotal"`
	MonitoringOOM      int       `json:"monitoringOOM"`
	MonitoringRestarts int       `json:"monitoringRestarts"`
	NotReadyNodes      []string  `json:"notReadyNodes,omitempty"`
}

// ClusterIncident is a notable transition during cleanup.
type ClusterIncident struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
	Node    string    `json:"node,omitempty"`
}

// CleanupListItem is a compact row for listing cleanup reports.
type CleanupListItem struct {
	ID         string              `json:"id"`
	Scope      string              `json:"scope"`
	Template   string              `json:"template,omitempty"`
	Cluster    string              `json:"cluster,omitempty"`
	Status     string              `json:"status"`
	DryRun     bool                `json:"dryRun"`
	Started    time.Time           `json:"started"`
	Finished   time.Time           `json:"finished"`
	Duration   string              `json:"duration"`
	DurationMs int64               `json:"durationMs"`
	DeletedNS  int                 `json:"deletedNamespaces"`
	Remaining  int                 `json:"remainingNamespaces"`
	Targeted   CleanupObjectTotals `json:"targeted"`
}

func cleanupReportsDir(runDir string) string {
	return filepath.Join(runDir, "cleanup-reports")
}

// CleanupReportID builds cleanup-{unix}-{scope}.
func CleanupReportID(scope string, finished time.Time) string {
	if finished.IsZero() {
		finished = time.Now()
	}
	s := scope
	if s == "" {
		s = "cleanup"
	}
	return fmt.Sprintf("cleanup-%d-%s", finished.Unix(), s)
}

// WriteCleanupReport freezes a cleanup report under {runDir}/cleanup-reports/{id}/.
func WriteCleanupReport(runDir string, doc *CleanupReport) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("nil cleanup report")
	}
	if doc.Finished.IsZero() {
		doc.Finished = time.Now()
	}
	if doc.Started.IsZero() {
		doc.Started = doc.Finished
	}
	d := doc.Finished.Sub(doc.Started)
	doc.DurationMs = d.Milliseconds()
	doc.Duration = d.Round(time.Second).String()
	if doc.ID == "" {
		doc.ID = CleanupReportID(doc.Scope, doc.Finished)
	}
	if doc.Warning == "" {
		doc.Warning = "NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT"
	}
	dir := filepath.Join(cleanupReportsDir(runDir), doc.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), b, 0o644); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(dir, "summary.json"), mustJSON(itemFromCleanup(doc)), 0o644)
	_ = os.WriteFile(filepath.Join(cleanupReportsDir(runDir), "latest.json"), b, 0o644)
	return doc.ID, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func itemFromCleanup(doc *CleanupReport) CleanupListItem {
	return CleanupListItem{
		ID:         doc.ID,
		Scope:      doc.Scope,
		Template:   doc.Template,
		Cluster:    doc.Cluster,
		Status:     doc.Status,
		DryRun:     doc.DryRun,
		Started:    doc.Started,
		Finished:   doc.Finished,
		Duration:   doc.Duration,
		DurationMs: doc.DurationMs,
		DeletedNS:  doc.DeletedNS,
		Remaining:  doc.Remaining,
		Targeted:   doc.Targeted,
	}
}

// LoadCleanupReport loads one cleanup report by id.
func LoadCleanupReport(runDir, id string) (*CleanupReport, error) {
	b, err := os.ReadFile(filepath.Join(cleanupReportsDir(runDir), id, "report.json"))
	if err != nil {
		return nil, err
	}
	var doc CleanupReport
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// LatestCleanupReport returns the most recently written cleanup report, if any.
func LatestCleanupReport(runDir string) (*CleanupReport, error) {
	b, err := os.ReadFile(filepath.Join(cleanupReportsDir(runDir), "latest.json"))
	if err != nil {
		return nil, err
	}
	var doc CleanupReport
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// ListCleanupReports returns newest-first summary rows.
func ListCleanupReports(runDir string) ([]CleanupListItem, error) {
	root := cleanupReportsDir(runDir)
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []CleanupListItem
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		item, err := loadCleanupListItem(runDir, e.Name())
		if err != nil {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Finished.After(out[j].Finished)
	})
	return out, nil
}

func loadCleanupListItem(runDir, id string) (CleanupListItem, error) {
	sumPath := filepath.Join(cleanupReportsDir(runDir), id, "summary.json")
	if b, err := os.ReadFile(sumPath); err == nil {
		var item CleanupListItem
		if json.Unmarshal(b, &item) == nil && item.ID != "" {
			return item, nil
		}
	}
	doc, err := LoadCleanupReport(runDir, id)
	if err != nil {
		return CleanupListItem{}, err
	}
	item := itemFromCleanup(doc)
	_ = os.WriteFile(sumPath, mustJSON(item), 0o644)
	return item, nil
}
