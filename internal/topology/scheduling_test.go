package topology

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/dasmlab/dasm-burner/internal/config"
)

func TestFilterTolerationsDropsInfra(t *testing.T) {
	avoid := config.DefaultAvoidTaints()
	in := []corev1.Toleration{
		{Key: "node-role.kubernetes.io", Operator: corev1.TolerationOpEqual, Value: "infra", Effect: corev1.TaintEffectNoSchedule},
		{Key: "node.kubernetes.io/unreachable", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
	}
	out := FilterTolerations(in, avoid)
	if len(out) != 1 || out[0].Key != "node.kubernetes.io/unreachable" {
		t.Fatalf("got %+v", out)
	}
}

func TestAvoidTaintAffinityInfra(t *testing.T) {
	aff := AvoidTaintAffinity(config.DefaultAvoidTaints())
	if aff == nil {
		t.Fatal("expected affinity")
	}
	exprs := aff.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	if len(exprs) < 2 {
		t.Fatalf("expected NotIn + DoesNotExist, got %+v", exprs)
	}
}

func TestApplyScheduling(t *testing.T) {
	spec := &corev1.PodSpec{
		Tolerations: []corev1.Toleration{
			{Key: "node-role.kubernetes.io", Operator: corev1.TolerationOpEqual, Value: "infra", Effect: corev1.TaintEffectNoSchedule},
		},
	}
	ApplyScheduling(spec, config.DefaultAvoidTaints())
	if len(spec.Tolerations) != 0 {
		t.Fatalf("tolerations should be stripped, got %+v", spec.Tolerations)
	}
	if spec.Affinity == nil || spec.Affinity.NodeAffinity == nil {
		t.Fatal("expected nodeAffinity")
	}
}
