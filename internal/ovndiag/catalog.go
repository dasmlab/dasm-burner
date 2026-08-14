package ovndiag

// Rule catalogue — expand as layers come online.
const (
	RuleNodeNotReady       = "OVN001"
	RuleMemoryPressure     = "OVN002"
	RuleDiskPressure       = "OVN003"
	RulePIDPressure        = "OVN004"
	RuleNetworkUnavailable = "OVN005"
	RuleReadyFlap          = "OVN006"

	RuleOVNKubeRestart   = "OVN101"
	RuleOVNKubeCrashLoop = "OVN102"
	RuleOVNKubeNotReady  = "OVN103"
	RuleOVNKubeOOM       = "OVN104"
	RuleEventBurst       = "OVN105"

	RuleResourceCPU    = "OVN301"
	RuleResourceMem    = "OVN302"
	RuleOVSProcessFail = "OVN303"

	RuleOVNDBNotReady = "OVN201"
	RuleOVNDBLatency  = "OVN202"

	RuleNetworkConfigDrift = "OVN401"
	RuleMissingAnnot       = "OVN402"
	RuleMissingSubnet      = "OVN403"

	RuleOVSNotReady     = "OVN501"
	RuleGatewayInvalid  = "OVN502"
	RuleSandboxFail     = "OVN503"
	RuleNoPodIP         = "OVN504"
	RulePacketDrop      = "OVN505"

	RuleLogAnomaly      = "OVN601"
	RuleErrorRateAccel  = "OVN602"
	RuleCorrelatedBatch = "OVN603"
)

// RuleInfo is human-readable context for a rule id (shown in the UI).
type RuleInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	About   string `json:"about"`
	Layer   string `json:"layer,omitempty"`
	Default string `json:"defaultSeverity,omitempty"`
}

// RuleCatalog maps rule IDs → short operator-facing explanations.
var RuleCatalog = map[string]RuleInfo{
	RuleNodeNotReady:       {ID: RuleNodeNotReady, Title: "Node NotReady", About: "Node Ready=False. OVN dataplane on this node is unreliable until Ready recovers.", Layer: "L1"},
	RuleMemoryPressure:     {ID: RuleMemoryPressure, Title: "MemoryPressure", About: "Node MemoryPressure=True — can starve ovnkube-node / OVS.", Layer: "L1"},
	RuleDiskPressure:       {ID: RuleDiskPressure, Title: "DiskPressure", About: "Node DiskPressure=True — risks OVN DB and log writes.", Layer: "L1"},
	RulePIDPressure:        {ID: RulePIDPressure, Title: "PIDPressure", About: "Node PIDPressure=True — can block OVN/OVS process recovery.", Layer: "L1"},
	RuleNetworkUnavailable: {ID: RuleNetworkUnavailable, Title: "NetworkUnavailable", About: "CNI reports network unavailable on this node.", Layer: "L1"},
	RuleReadyFlap:          {ID: RuleReadyFlap, Title: "Ready flap vs baseline", About: "Node was NotReady at baseline and is Ready now (or the reverse path was observed).", Layer: "L1"},

	RuleOVNKubeRestart:   {ID: RuleOVNKubeRestart, Title: "ovnkube-node restart Δ", About: "Container restart count rose vs the captured baseline watermark.", Layer: "L2"},
	RuleOVNKubeCrashLoop: {ID: RuleOVNKubeCrashLoop, Title: "CrashLoopBackOff", About: "ovnkube-node container waiting in CrashLoopBackOff.", Layer: "L2"},
	RuleOVNKubeNotReady:  {ID: RuleOVNKubeNotReady, Title: "ovnkube-node not Ready", About: "ovnkube-node Ready=False or missing on the node.", Layer: "L2"},
	RuleOVNKubeOOM:       {ID: RuleOVNKubeOOM, Title: "OOMKilled", About: "Last termination reason was OOMKilled.", Layer: "L2"},
	RuleEventBurst:       {ID: RuleEventBurst, Title: "Warning event burst", About: "Aggregated Warning events in openshift-ovn-kubernetes during the sample window.", Layer: "L2"},

	RuleOVNDBNotReady: {ID: RuleOVNDBNotReady, Title: "OVN DB container not Ready", About: "nbdb / sbdb / northd container Ready=False.", Layer: "L4"},
	RuleOVNDBLatency:  {ID: RuleOVNDBLatency, Title: "OVN DB latency", About: "Reserved for transaction latency signals when available.", Layer: "L4"},

	RuleResourceCPU:    {ID: RuleResourceCPU, Title: "CPU elevated vs baseline", About: "Pod metrics CPU rose sharply vs baseline watermark.", Layer: "L3"},
	RuleResourceMem:    {ID: RuleResourceMem, Title: "Memory elevated vs baseline", About: "Pod metrics memory rose sharply vs baseline watermark.", Layer: "L3"},
	RuleOVSProcessFail: {ID: RuleOVSProcessFail, Title: "ovn-controller / OVS process", About: "ovn-controller (or related) container not Ready — dataplane programming risk.", Layer: "L4"},

	RuleNetworkConfigDrift: {ID: RuleNetworkConfigDrift, Title: "OVN annotation drift", About: "Expected k8s.ovn.org/* node annotations are missing or incomplete.", Layer: "L5"},
	RuleMissingAnnot:       {ID: RuleMissingAnnot, Title: "Missing OVN annotation", About: "A required OVN node annotation key is absent.", Layer: "L5"},
	RuleMissingSubnet:      {ID: RuleMissingSubnet, Title: "Missing node subnet", About: "k8s.ovn.org/node-subnets is empty.", Layer: "L5"},

	RuleOVSNotReady:    {ID: RuleOVSNotReady, Title: "OVS daemon not Ready", About: "ovs-daemons / ovs-vswitchd container not Ready — br-int / Geneve dataplane stalls.", Layer: "L6"},
	RuleGatewayInvalid: {ID: RuleGatewayInvalid, Title: "L3 gateway config incomplete", About: "k8s.ovn.org/l3-gateway-config missing mode, next-hop, or IP.", Layer: "L6"},
	RuleSandboxFail:    {ID: RuleSandboxFail, Title: "FailedCreatePodSandBox", About: "Kubelet could not set up the pod network sandbox (CNI/OVN) on this node.", Layer: "L6"},
	RuleNoPodIP:        {ID: RuleNoPodIP, Title: "Pending without PodIP", About: "Scheduled Pending pods have no CNI address — dataplane setup incomplete.", Layer: "L6"},
	RulePacketDrop:     {ID: RulePacketDrop, Title: "Packet drop / conntrack pressure", About: "OVN/OVS logs mention packet drops or conntrack table pressure.", Layer: "L6"},

	RuleLogAnomaly:      {ID: RuleLogAnomaly, Title: "OVN/OVS log class hit", About: "Normalized log scanner matched a class (ERROR, TIMEOUT, IPTABLES, …) in ovnkube-node logs. Not a raw log dump — use the sample line as the clue.", Layer: "logs"},
	RuleErrorRateAccel:  {ID: RuleErrorRateAccel, Title: "Elevated ERROR log rate", About: "Many ERROR-class log lines in a short window on an ovnkube-node pod.", Layer: "logs"},
	RuleCorrelatedBatch: {ID: RuleCorrelatedBatch, Title: "Batch-correlated degradation", About: "Multiple finding categories on the same node near a batch marker (OVN603).", Layer: "L7"},
}

func DescribeRule(id string) RuleInfo {
	if info, ok := RuleCatalog[id]; ok {
		return info
	}
	return RuleInfo{ID: id, Title: id, About: "No catalog entry yet for this rule."}
}
