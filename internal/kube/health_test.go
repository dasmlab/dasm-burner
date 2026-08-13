package kube

import "testing"

func TestApplyOVNRestartDeltas(t *testing.T) {
	open := Health{OVNDetail: []OVNPodDetail{
		{Name: "ovnkube-node-a", Node: "n1", Restarts: 40, Ready: true},
		{Name: "ovnkube-node-b", Node: "n2", Restarts: 5, Ready: true},
	}}
	close := Health{OVNDetail: []OVNPodDetail{
		{Name: "ovnkube-node-a", Node: "n1", Restarts: 42, Ready: true},
		{Name: "ovnkube-node-b", Node: "n2", Restarts: 5, Ready: false},
		{Name: "ovnkube-node-c", Node: "n3", Restarts: 9, Ready: true}, // new during run
	}}
	out := ApplyOVNRestartDeltas(open, close)
	if out.OVNRestartsDelta != 2 {
		t.Fatalf("delta sum=%d want 2", out.OVNRestartsDelta)
	}
	if out.OVNDetail[0].RestartsDelta != 2 {
		t.Fatalf("a delta=%d", out.OVNDetail[0].RestartsDelta)
	}
	if out.OVNDetail[2].RestartsDelta != 0 {
		t.Fatalf("new pod should not inherit lifetime as delta")
	}
}
