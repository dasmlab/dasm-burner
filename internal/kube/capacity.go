package kube

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/dasmlab/dasm-burner/internal/config"
)

// DensityCapacity compares asked pods to schedulable density slots
// (Ready nodes that burn pods can land on given avoid-taints).
type DensityCapacity struct {
	WorkerNodes    int  `json:"workerNodes"`
	Slots          int  `json:"slots"`
	MaxPodsTypical int  `json:"maxPodsTypical"`
	ExcludedNodes  int  `json:"excludedNodes"`
	PodsAsked      int  `json:"podsAsked"`
	WavePods       int  `json:"wavePods"`
	WaveNS         int  `json:"waveNamespaces"`
	FitsRun        bool `json:"fitsRun"`
	FitsWave       bool `json:"fitsWave"`
	Summary        string `json:"summary"`
}

// EvaluateDensityCapacity sums allocatable.pods on nodes that density pods
// can schedule onto (Ready; not excluded by avoid-infra affinity; no
// untolerated NoSchedule/NoExecute taints).
func EvaluateDensityCapacity(ctx context.Context, cs kubernetes.Interface, avoid []config.AvoidTaint, podsAsked, waveNS, podsPerNS int) (DensityCapacity, error) {
	out := DensityCapacity{PodsAsked: podsAsked, WaveNS: waveNS}
	if podsPerNS > 0 && waveNS > 0 {
		out.WavePods = waveNS * podsPerNS
	}
	if cs == nil {
		return out, fmt.Errorf("no kubernetes client")
	}
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return out, err
	}
	maxSeen := 0
	for _, n := range nodes.Items {
		if !nodeReady(n) {
			out.ExcludedNodes++
			continue
		}
		if densityNodeExcluded(n, avoid) {
			out.ExcludedNodes++
			continue
		}
		slots := allocatablePods(n)
		if slots <= 0 {
			out.ExcludedNodes++
			continue
		}
		out.WorkerNodes++
		out.Slots += slots
		if slots > maxSeen {
			maxSeen = slots
		}
	}
	out.MaxPodsTypical = maxSeen
	out.FitsRun = out.PodsAsked <= out.Slots
	out.FitsWave = out.WavePods == 0 || out.WavePods <= out.Slots
	out.Summary = formatCapacitySummary(out)
	return out, nil
}

func formatCapacitySummary(c DensityCapacity) string {
	typ := c.MaxPodsTypical
	if typ <= 0 {
		typ = 0
	}
	base := fmt.Sprintf("density slots=%d (%d schedulable nodes × ~%d maxPods) · run wants %d pods",
		c.Slots, c.WorkerNodes, typ, c.PodsAsked)
	if c.WavePods > 0 {
		base += fmt.Sprintf(" · largest wave ~%d pods (%d NS)", c.WavePods, c.WaveNS)
	}
	switch {
	case !c.FitsRun:
		base += " · RUN EXCEEDS SLOTS"
	case !c.FitsWave:
		base += " · WAVE EXCEEDS SLOTS"
	default:
		base += " · fits"
	}
	return base
}

func allocatablePods(n corev1.Node) int {
	raw, ok := n.Status.Allocatable[corev1.ResourcePods]
	if !ok {
		return 0
	}
	v := raw.Value()
	if v > 0 {
		return int(v)
	}
	// Quantity may be decimal string in some fakes
	if s := raw.String(); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return 0
}

func densityNodeExcluded(n corev1.Node, avoid []config.AvoidTaint) bool {
	labels := n.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	// Hard rule: never count master/control-plane toward density slots.
	if _, ok := labels["node-role.kubernetes.io/master"]; ok {
		return true
	}
	if _, ok := labels["node-role.kubernetes.io/control-plane"]; ok {
		return true
	}
	// Avoid-infra affinity: skip nodes carrying avoided role labels.
	for _, a := range avoid {
		switch {
		case a.Key == "node-role.kubernetes.io" && a.Value == "infra":
			if _, ok := labels["node-role.kubernetes.io/infra"]; ok {
				return true
			}
			if labels["node-role.kubernetes.io"] == "infra" {
				return true
			}
		case a.Key != "" && a.Value == "":
			if _, ok := labels[a.Key]; ok {
				return true
			}
		case a.Key != "" && a.Value != "":
			if labels[a.Key] == a.Value {
				return true
			}
		}
	}
	// Pods have no master/control-plane tolerations — skip tainted control plane.
	for _, t := range n.Spec.Taints {
		if t.Effect != corev1.TaintEffectNoSchedule && t.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		if t.Key == "node-role.kubernetes.io/master" || t.Key == "node-role.kubernetes.io/control-plane" {
			return true
		}
		// Infra / avoided NoSchedule (even if affinity already caught the label).
		for _, a := range avoid {
			if avoidMatchesNodeTaint(a, t) {
				return true
			}
		}
	}
	return false
}

func avoidMatchesNodeTaint(a config.AvoidTaint, t corev1.Taint) bool {
	if a.Key == "" {
		return false
	}
	// kubectl form node-role.kubernetes.io=infra vs OCP node-role.kubernetes.io/infra
	if a.Key == "node-role.kubernetes.io" && a.Value == "infra" {
		if t.Key == "node-role.kubernetes.io" && t.Value == "infra" {
			return effectCompatible(a.Effect, string(t.Effect))
		}
		if t.Key == "node-role.kubernetes.io/infra" {
			return effectCompatible(a.Effect, string(t.Effect))
		}
		return false
	}
	if t.Key != a.Key {
		return false
	}
	if a.Value != "" && t.Value != a.Value {
		return false
	}
	return effectCompatible(a.Effect, string(t.Effect))
}

func effectCompatible(avoidEffect, taintEffect string) bool {
	if avoidEffect == "" || taintEffect == "" {
		return true
	}
	return avoidEffect == taintEffect
}

// CapacityExceededError is returned when the run/wave does not fit density slots.
type CapacityExceededError struct {
	Capacity DensityCapacity
	Message  string
}

func (e *CapacityExceededError) Error() string {
	if e == nil {
		return "capacity exceeded"
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Capacity.Summary
}

// CheckDensityFit returns a CapacityExceededError when the full run (or largest
// wave) needs more pods than density slots. allowOver skips the hard fail.
func CheckDensityFit(cap DensityCapacity, allowOver bool) error {
	if allowOver || (cap.FitsRun && cap.FitsWave) {
		return nil
	}
	msg := fmt.Sprintf(
		"capacity: run wants %d pods; density slots ~%d (%d nodes × ~%d maxPods). largest wave ~%d pods (%d NS). "+
			"Raise maxPods (KubeletConfig), add workers, lower replicasPerService, or shrink batch. "+
			"Re-submit with allowOverCapacity=true to proceed anyway.",
		cap.PodsAsked, cap.Slots, cap.WorkerNodes, cap.MaxPodsTypical, cap.WavePods, cap.WaveNS,
	)
	return &CapacityExceededError{Capacity: cap, Message: msg}
}
