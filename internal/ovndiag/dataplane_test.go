package ovndiag

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseGatewayConfig(t *testing.T) {
	raw := `{"default":{"mode":"shared","ip-addresses":["192.168.10.12/24"],"next-hop":"192.168.10.1"}}`
	ok, mode := parseGatewayConfig(raw)
	if !ok || mode != "shared" {
		t.Fatalf("ok=%v mode=%q", ok, mode)
	}
	ok, _ = parseGatewayConfig(`{"default":{"mode":"local"}}`)
	if ok {
		t.Fatal("incomplete gateway should fail")
	}
	ok, _ = parseGatewayConfig("")
	if ok {
		t.Fatal("empty should fail")
	}
}

func TestEvaluateDataplaneSandbox(t *testing.T) {
	now := time.Now()
	n := OVNNodeHealth{
		NodeName: "worker-1",
		Dataplane: DataplaneLayer{
			Present: true, OVSReady: true, GatewayOK: true, SandboxFailures: 6,
		},
	}
	sig := dataplaneSignals{sandbox: map[string]nodeSignal{
		"worker-1": {Count: 6, Sample: "failed to set up pod network"},
	}}
	fs := evaluateDataplane(n, sig, now, 3)
	if len(fs) != 1 || fs[0].RuleID != RuleSandboxFail || fs[0].Severity != SevError {
		t.Fatalf("unexpected %+v", fs)
	}
}

func TestOVSDaemonStatus(t *testing.T) {
	pods := []corev1.Pod{{
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: "ovn-controller", Ready: true},
			{Name: "ovs-daemons", Ready: false},
		}},
	}}
	ready, present := ovsDaemonStatus(pods)
	if !present || ready {
		t.Fatalf("present=%v ready=%v", present, ready)
	}
}

func TestPendingLooksNetwork(t *testing.T) {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-0"},
		Spec:       corev1.PodSpec{NodeName: "w1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionTrue,
			}},
		},
	}
	if !pendingLooksNetwork(p) {
		t.Fatal("scheduled pending without IP should count")
	}
}
