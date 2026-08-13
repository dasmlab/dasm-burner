package config

import "testing"

func TestParseAvoidTaint(t *testing.T) {
	cases := []struct {
		in   string
		want AvoidTaint
	}{
		{"node-role.kubernetes.io=infra:NoSchedule", AvoidTaint{Key: "node-role.kubernetes.io", Value: "infra", Effect: "NoSchedule"}},
		{"node-role.kubernetes.io/infra:NoSchedule", AvoidTaint{Key: "node-role.kubernetes.io/infra", Value: "", Effect: "NoSchedule"}},
		{"node-role.kubernetes.io=infra", AvoidTaint{Key: "node-role.kubernetes.io", Value: "infra", Effect: "NoSchedule"}},
		{"dedicated=gpu:NoExecute", AvoidTaint{Key: "dedicated", Value: "gpu", Effect: "NoExecute"}},
	}
	for _, tc := range cases {
		got, err := ParseAvoidTaint(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %+v want %+v", tc.in, got, tc.want)
		}
	}
}

func TestMatchesToleration(t *testing.T) {
	avoid := AvoidTaint{Key: "node-role.kubernetes.io", Value: "infra", Effect: "NoSchedule"}
	if !avoid.MatchesToleration("node-role.kubernetes.io", "infra", "NoSchedule", "Equal") {
		t.Fatal("expected match for equal infra toleration")
	}
	if !avoid.MatchesToleration("node-role.kubernetes.io", "", "NoSchedule", "Exists") {
		t.Fatal("expected match for Exists on same key")
	}
	if avoid.MatchesToleration("node-role.kubernetes.io", "worker", "NoSchedule", "Equal") {
		t.Fatal("worker value should not match infra avoid")
	}
	if !avoid.MatchesToleration("", "", "", "") {
		t.Fatal("empty-key wildcard toleration must match")
	}
}
