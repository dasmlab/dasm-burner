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

	RuleCorrelatedBatch = "OVN603"
)
