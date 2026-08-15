package sourcemap

// Isolation is the north-star run mode: one wave, then recover, then the next.
var Isolation = Protocol{
	Name:           "Isolated Wave Test Approach / Mode",
	NorthStar:      "Find which wave creates RSS, whether it ever comes back, and the causality chain across kube-apiserver, etcd, and OVN. A 14-wave dump cannot do that.",
	BreakpointHint: "On the ~15k-pod / 3-replica template the cliff has shown up around B6. That is a hypothesis until each wave is its own closed loop.",
	Causality: []string{
		"API watch-cache alloc",
		"LIST / inflight",
		"etcd write / watch",
		"master RAM",
		"oauth replica on that master",
		"OVN reconnect",
	},
	Steps: []Step{
		{ID: "baseline", Title: "Baseline", See: "3/3 Ready. kas / etcd / OVN RSS, etcd DB bytes, oauth 3/3. Hold until RSS is flat — not merely until pods are Running."},
		{ID: "apply", Title: "Apply only wave k", See: "Same batch size as today (~187 NS, 3-replica). No B{k+1}."},
		{ID: "settle", Title: "Settle", See: "Keep sampling. Record Δ RSS, LIST latency, inflight, etcd apply lag, ovn-node RSS."},
		{ID: "delete", Title: "Delete that wave only", See: "Same pace as create. Sample the way down. When Terminating hits 0: did kas RSS drop, or only after a static-pod restart?"},
		{ID: "giveback", Title: "Give-back", See: "Objects gone vs RSS. Leftover floor is a finding (ETCD008), not a bug in the test."},
		{ID: "reset", Title: "Reset / next k", See: "Accept leftover RSS as the new floor, or restart kube-apiserver for a cold baseline. Repeat B0, B1, … until a wave does not settle — that wave is the breakpoint."},
	},
}

type Protocol struct {
	Name           string   `json:"name"`
	NorthStar      string   `json:"northStar"`
	BreakpointHint string   `json:"breakpointHint"`
	Causality      []string `json:"causality"`
	Steps          []Step   `json:"steps"`
}

type Step struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	See   string `json:"see"`
}
