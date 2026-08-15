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
	Name              string  `json:"name"`
	Ready             bool    `json:"ready"`
	MemoryPressure    bool    `json:"memoryPressure"`
	DiskPressure      bool    `json:"diskPressure"`
	PIDPressure       bool    `json:"pidPressure"`
	EtcdPod           string  `json:"etcdPod,omitempty"`
	EtcdReady         bool    `json:"etcdReady"`
	EtcdRestarts      int     `json:"etcdRestarts"`
	EtcdRSSMi         float64 `json:"etcdRssMi,omitempty"`
	APIServerPod      string  `json:"apiserverPod,omitempty"`
	APIServerReady    bool    `json:"apiserverReady"`
	APIServerRestarts int     `json:"apiserverRestarts,omitempty"`
	APIRSSMi          float64 `json:"apiserverRssMi,omitempty"`
	OVNPod            string  `json:"ovnPod,omitempty"`
	OVNRSSMi          float64 `json:"ovnRssMi,omitempty"`
}

type EtcdMember struct {
	PodName  string `json:"podName"`
	Node     string `json:"node,omitempty"`
	Ready    bool   `json:"ready"`
	Restarts int    `json:"restarts"`
	Phase    string `json:"phase,omitempty"`
}

// Cascade is the observed control-plane failure order under density.
// Observed on TEST3: kube-apiserver RSS/LIST flex → etcd timeouts → masters/OVN.
const (
	StageIdle     = "idle"
	StageAPIFlex  = "api_flex"
	StageEtcdFlex = "etcd_flex"
	StageCollapse = "collapse"
	StageLeftover = "leftover" // workload gone, API RSS still fat
)

type Snapshot struct {
	GeneratedAt       time.Time    `json:"generatedAt"`
	BaselineAt        time.Time    `json:"baselineAt,omitempty"`
	RunID             string       `json:"runId,omitempty"`
	Cluster           string       `json:"cluster,omitempty"`
	BatchID           int          `json:"batchId,omitempty"`
	OverallState      HealthState  `json:"overallState"`
	Cascade           string       `json:"cascade,omitempty"`
	CascadeWhy        string       `json:"cascadeWhy,omitempty"`
	MastersReady      int          `json:"mastersReady"`
	MastersTotal      int          `json:"mastersTotal"`
	EtcdReady         int          `json:"etcdReady"`
	EtcdTotal         int          `json:"etcdTotal"`
	APIReady          int          `json:"apiserverReady"`
	APITotal          int          `json:"apiserverTotal"`
	MemPressure       int          `json:"mastersMemoryPressure"`
	WorkloadPods      int          `json:"workloadPods"`
	WorkloadNS        int          `json:"workloadNamespaces"`
	WorkerNodes       int          `json:"workerNodes,omitempty"`
	MaxPodsTypical    int          `json:"maxPodsTypical,omitempty"`
	MetricsOK         bool         `json:"metricsOk"`
	APIRSSMi          float64      `json:"apiserverRssMi,omitempty"`
	EtcdRSSMi         float64      `json:"etcdRssMi,omitempty"`
	OVNRSSMi          float64      `json:"ovnRssMi,omitempty"`
	BaselineAPIRSSMi  float64      `json:"baselineApiserverRssMi,omitempty"`
	BaselineEtcdRSSMi float64      `json:"baselineEtcdRssMi,omitempty"`
	BaselineOVNRSSMi  float64      `json:"baselineOvnRssMi,omitempty"`
	FindingCount      int          `json:"findingCount"`
	CriticalCount     int          `json:"criticalCount"`
	WarningCount      int          `json:"warningCount"`
	HealthyCount      int          `json:"healthyCount"`
	Masters           []MasterNode `json:"masters,omitempty"`
	Etcd              []EtcdMember `json:"etcd,omitempty"`
	Findings          []Finding    `json:"findings,omitempty"`
	Why               string       `json:"why,omitempty"`
	WhyLines          []string     `json:"whyLines,omitempty"`
	Kind              string       `json:"kind,omitempty"` // baseline | sample
}

// SeriesPoint is one PVC-backed sample for the RSS / cascade graph.
type SeriesPoint struct {
	At           time.Time `json:"at"`
	ID           string    `json:"id,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	BatchID      int       `json:"batchId,omitempty"`
	RunID        string    `json:"runId,omitempty"`
	Cascade      string    `json:"cascade,omitempty"`
	WorkloadPods int       `json:"workloadPods"`
	WorkloadNS   int       `json:"workloadNamespaces"`
	APIRSSMi     float64   `json:"apiserverRssMi"`
	EtcdRSSMi    float64   `json:"etcdRssMi"`
	OVNRSSMi     float64   `json:"ovnRssMi"`
	APIReady     int       `json:"apiserverReady"`
	APITotal     int       `json:"apiserverTotal"`
	EtcdReady    int       `json:"etcdReady"`
	EtcdTotal    int       `json:"etcdTotal"`
	MastersReady int       `json:"mastersReady"`
	MastersTotal int       `json:"mastersTotal"`
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
	"ETCD004": {ID: "ETCD004", Title: "etcd member restarts vs baseline", About: "etcd restart count rose since baseline — not a scar from an older cliff."},
	"ETCD005": {ID: "ETCD005", Title: "kube-apiserver not Ready", About: "A static kube-apiserver pod on a master is not Ready."},
	"ETCD006": {ID: "ETCD006", Title: "etcd quorum risk", About: "Fewer than majority of etcd members are Ready."},
	"ETCD007": {ID: "ETCD007", Title: "kube-apiserver RSS vs baseline", About: "API working set grew under load. This is the first flex in the density cascade."},
	"ETCD008": {ID: "ETCD008", Title: "Leftover API RSS after cleanup", About: "Workload objects are gone but kube-apiserver RSS did not return to baseline. watch_cache.Delete still appends a Deleted event; resizeCacheLocked only shrinks when the ring is full. Go also will not return RSS to the OS until the static pod restarts."},
	"ETCD009": {ID: "ETCD009", Title: "Cascade advanced", About: "Observed order: kube-apiserver flex → etcd flex → master/OVN collapse."},
	"ETCD010": {ID: "ETCD010", Title: "kube-apiserver restarts vs baseline", About: "API static pod restarted since baseline — RSS may have reset then grown again."},
}
