package ovndiag

import "time"

// HealthState is the diagnoser state machine.
type HealthState string

const (
	StateHealthy  HealthState = "HEALTHY"
	StateDegraded HealthState = "DEGRADED"
	StateWarning  HealthState = "WARNING"
	StateCritical HealthState = "CRITICAL"
	StateFailed   HealthState = "FAILED"
)

type Severity string

const (
	SevInfo     Severity = "INFO"
	SevNotice   Severity = "NOTICE"
	SevWarning  Severity = "WARNING"
	SevError    Severity = "ERROR"
	SevCritical Severity = "CRITICAL"
)

type Category string

const (
	CatNode      Category = "NODE"
	CatOVNKube   Category = "OVNKUBE"
	CatOVN       Category = "OVN"
	CatOVS       Category = "OVS"
	CatDatabase  Category = "DATABASE"
	CatNetwork   Category = "NETWORK"
	CatDataplane Category = "DATAPLANE"
	CatResource  Category = "RESOURCE"
	CatLog       Category = "LOG"
	CatCorrelate Category = "CORRELATE"
)

type Evidence struct {
	Label    string            `json:"label"`
	Baseline string            `json:"baseline,omitempty"`
	Current  string            `json:"current,omitempty"`
	Delta    string            `json:"delta,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

type Finding struct {
	ID        string     `json:"id"`
	RuleID    string     `json:"ruleId"`
	Severity  Severity   `json:"severity"`
	Category  Category   `json:"category"`
	Node      string     `json:"node,omitempty"`
	Component string     `json:"component,omitempty"`
	FirstSeen time.Time  `json:"firstSeen"`
	LastSeen  time.Time  `json:"lastSeen"`
	Count     int        `json:"count"`
	Summary   string     `json:"summary"`
	Evidence  []Evidence `json:"evidence,omitempty"`
	BatchID   int        `json:"batchId,omitempty"`
	Why       string     `json:"why,omitempty"`
}

type TimelineEvent struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"` // batch | finding | state
	Summary  string    `json:"summary"`
	Node     string    `json:"node,omitempty"`
	BatchID  int       `json:"batchId,omitempty"`
	Finding  string    `json:"findingId,omitempty"`
	Severity Severity  `json:"severity,omitempty"`
}

type NodeLayer struct {
	Ready              bool      `json:"ready"`
	MemoryPressure     bool      `json:"memoryPressure"`
	DiskPressure       bool      `json:"diskPressure"`
	PIDPressure        bool      `json:"pidPressure"`
	NetworkUnavailable bool      `json:"networkUnavailable"`
	ReadyTransitions   int       `json:"readyTransitions"`
	LastReadyChange    time.Time `json:"lastReadyChange,omitempty"`
}

type OVNKubeLayer struct {
	PodName       string `json:"podName,omitempty"`
	Phase         string `json:"phase,omitempty"`
	Ready         bool   `json:"ready"`
	Restarts      int    `json:"restarts"`
	RestartsDelta int    `json:"restartsDelta"`
	OOMKilled     bool   `json:"oomKilled"`
	CrashLoop     bool   `json:"crashLoop"`
}

type OVNNodeHealth struct {
	NodeName     string       `json:"nodeName"`
	OverallState HealthState  `json:"overallState"`
	Node         NodeLayer    `json:"node"`
	OVNKube      OVNKubeLayer `json:"ovnKube"`
	Findings     []Finding    `json:"findings,omitempty"`
}

// Snapshot is an immutable diagnoser view for a moment (or end-of-run freeze).
type Snapshot struct {
	GeneratedAt   time.Time       `json:"generatedAt"`
	RunID         string          `json:"runId,omitempty"`
	Cluster       string          `json:"cluster,omitempty"`
	OverallState  HealthState     `json:"overallState"`
	BaselineAt    time.Time       `json:"baselineAt,omitempty"`
	Nodes         []OVNNodeHealth `json:"nodes"`
	Findings      []Finding       `json:"findings,omitempty"`
	Timeline      []TimelineEvent `json:"timeline,omitempty"`
	Why           string          `json:"why,omitempty"`
	Capabilities  map[string]bool `json:"capabilities,omitempty"`
	HealthyCount  int             `json:"healthyCount"`
	WarningCount  int             `json:"warningCount"`
	CriticalCount int             `json:"criticalCount"`
	BatchMarkers  []TimelineEvent `json:"batchMarkers,omitempty"`
}
