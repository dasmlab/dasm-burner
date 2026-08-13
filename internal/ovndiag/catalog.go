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

	RuleLogAnomaly      = "OVN601"
	RuleErrorRateAccel  = "OVN602"
	RuleCorrelatedBatch = "OVN603"
)
