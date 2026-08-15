package etcddiag

import "time"

type HealthState string

const (
	StateHealthy  HealthState = "HEALTHY"
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

type Finding struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"ruleId"`
	Severity  Severity  `json:"severity"`
	Node      string    `json:"node,omitempty"`
	Component string    `json:"component,omitempty"`
	Summary   string    `json:"summary"`
	Why       string    `json:"why,omitempty"`
	BatchID   int       `json:"batchId,omitempty"`
	At        time.Time `json:"at"`
}

type MasterNode struct {
	Name           string `json:"name"`
	Ready          bool   `json:"ready"`
	MemoryPressure bool   `json:"memoryPressure"`
	DiskPressure   bool   `json:"diskPressure"`
	PIDPressure    bool   `json:"pidPressure"`
	EtcdPod        string `json:"etcdPod,omitempty"`
	EtcdReady      bool   `json:"etcdReady"`
	EtcdRestarts   int    `json:"etcdRestarts"`
	APIServerPod   string `json:"apiserverPod,omitempty"`
	APIServerReady bool   `json:"apiserverReady"`
}

type EtcdMember struct {
	PodName  string `json:"podName"`
	Node     string `json:"node,omitempty"`
	Ready    bool   `json:"ready"`
	Restarts int    `json:"restarts"`
	Phase    string `json:"phase,omitempty"`
}

type Snapshot struct {
	GeneratedAt   time.Time    `json:"generatedAt"`
	BaselineAt    time.Time    `json:"baselineAt,omitempty"`
	RunID         string       `json:"runId,omitempty"`
	Cluster       string       `json:"cluster,omitempty"`
	BatchID       int          `json:"batchId,omitempty"`
	OverallState  HealthState  `json:"overallState"`
	MastersReady  int          `json:"mastersReady"`
	MastersTotal  int          `json:"mastersTotal"`
	EtcdReady     int          `json:"etcdReady"`
	EtcdTotal     int          `json:"etcdTotal"`
	APIReady      int          `json:"apiserverReady"`
	APITotal      int          `json:"apiserverTotal"`
	MemPressure   int          `json:"mastersMemoryPressure"`
	FindingCount  int          `json:"findingCount"`
	CriticalCount int          `json:"criticalCount"`
	WarningCount  int          `json:"warningCount"`
	HealthyCount  int          `json:"healthyCount"`
	Masters       []MasterNode `json:"masters,omitempty"`
	Etcd          []EtcdMember `json:"etcd,omitempty"`
	Findings      []Finding    `json:"findings,omitempty"`
	Why           string       `json:"why,omitempty"`
	WhyLines      []string     `json:"whyLines,omitempty"`
	Kind          string       `json:"kind,omitempty"` // baseline | sample
}

type RuleInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	About string `json:"about"`
}

var RuleCatalog = map[string]RuleInfo{
	"ETCD001": {ID: "ETCD001", Title: "Control-plane node NotReady", About: "A master/control-plane node stopped posting Ready — etcd/API heartbeats fail next."},
	"ETCD002": {ID: "ETCD002", Title: "etcd static pod not Ready", About: "An openshift-etcd member pod is not Ready."},
	"ETCD003": {ID: "ETCD003", Title: "Master MemoryPressure", About: "Control-plane node reports MemoryPressure — etcd and apiserver compete for RAM."},
	"ETCD004": {ID: "ETCD004", Title: "etcd member restarts", About: "etcd pod restart count is elevated vs a quiet baseline."},
	"ETCD005": {ID: "ETCD005", Title: "kube-apiserver not Ready", About: "A static kube-apiserver pod on a master is not Ready."},
	"ETCD006": {ID: "ETCD006", Title: "etcd quorum risk", About: "Fewer than majority of etcd members are Ready."},
}
