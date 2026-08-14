package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dasmlab/dasm-burner/internal/config"
)

func TestDensityNodeExcluded(t *testing.T) {
	t.Parallel()
	avoid := config.DefaultAvoidTaints()

	worker := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "w1", Labels: map[string]string{"node-role.kubernetes.io/worker": ""}},
	}
	if densityNodeExcluded(worker, avoid) {
		t.Fatal("worker should be included")
	}

	infra := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "i1", Labels: map[string]string{
			"node-role.kubernetes.io/infra":  "",
			"node-role.kubernetes.io/worker": "",
		}},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
			Key: "node-role.kubernetes.io", Value: "infra", Effect: corev1.TaintEffectNoSchedule,
		}}},
	}
	if !densityNodeExcluded(infra, avoid) {
		t.Fatal("infra should be excluded")
	}

	master := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Labels: map[string]string{"node-role.kubernetes.io/master": ""}},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
			Key: "node-role.kubernetes.io/master", Effect: corev1.TaintEffectNoSchedule,
		}}},
	}
	if !densityNodeExcluded(master, avoid) {
		t.Fatal("master should be excluded")
	}
}

func TestCheckDensityFit(t *testing.T) {
	t.Parallel()
	cap := DensityCapacity{
		WorkerNodes: 15, Slots: 3750, MaxPodsTypical: 250,
		PodsAsked: 15000, WavePods: 1878, WaveNS: 313,
		FitsRun: false, FitsWave: true,
	}
	err := CheckDensityFit(cap, false)
	if err == nil {
		t.Fatal("expected capacity error")
	}
	ce, ok := err.(*CapacityExceededError)
	if !ok {
		t.Fatalf("want CapacityExceededError, got %T", err)
	}
	if ce.Capacity.Slots != 3750 {
		t.Fatalf("slots=%d", ce.Capacity.Slots)
	}
	if CheckDensityFit(cap, true) != nil {
		t.Fatal("allowOver should skip")
	}
	cap.FitsRun = true
	cap.PodsAsked = 3000
	if CheckDensityFit(cap, false) != nil {
		t.Fatal("fitting run should pass")
	}
}

func TestAllocatablePods(t *testing.T) {
	t.Parallel()
	n := corev1.Node{Status: corev1.NodeStatus{
		Allocatable: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("500")},
	}}
	if got := allocatablePods(n); got != 500 {
		t.Fatalf("got %d", got)
	}
}
