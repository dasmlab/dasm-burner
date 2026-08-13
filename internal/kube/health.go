package kube

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

const ovnNamespace = "openshift-ovn-kubernetes"

// Health is a cluster snapshot used for abort gates and the OVN report.
type Health struct {
	SampledAt     time.Time `json:"sampledAt"`
	NodesReady    int       `json:"nodesReady"`
	NodesNotReady int       `json:"nodesNotReady"`
	NotReadyNodes []string  `json:"notReadyNodes,omitempty"`

	ManagedPods  int `json:"managedPods"`
	ManagedReady int `json:"managedReadyPods"`
	FailedPods   int `json:"failedPods"`
	OOMKilled    int `json:"oomKilled"`

	OVNPods     int `json:"ovnPods"`
	OVNReady    int `json:"ovnReady"`
	OVNRestarts int `json:"ovnRestarts"`

	WarningEvents int `json:"warningEvents"`
	ErrorEvents   int `json:"errorEvents"`
}

func (l *Live) ClusterHealth(ctx context.Context, runID string) (Health, error) {
	h := Health{SampledAt: time.Now()}

	nodes, err := l.cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return h, err
	}
	for _, n := range nodes.Items {
		if nodeReady(n) {
			h.NodesReady++
		} else {
			h.NodesNotReady++
			h.NotReadyNodes = append(h.NotReadyNodes, n.Name)
		}
	}

	sel := topology.Selector(runID)
	pods, err := l.cs.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return h, err
	}
	for _, p := range pods.Items {
		h.ManagedPods++
		if podReady(p) {
			h.ManagedReady++
		}
		if p.Status.Phase == corev1.PodFailed {
			h.FailedPods++
		}
		if oomKilled(p) {
			h.OOMKilled++
		}
	}

	ovn, err := l.cs.CoreV1().Pods(ovnNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, p := range ovn.Items {
			h.OVNPods++
			if podReady(p) {
				h.OVNReady++
			}
			h.OVNRestarts += restartCount(p)
		}
	}

	cutoff := time.Now().Add(-10 * time.Minute)
	for _, ns := range []string{ovnNamespace, "openshift-etcd", "openshift-kube-apiserver"} {
		evs, err := l.cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{Limit: 200})
		if err != nil {
			continue
		}
		for _, e := range evs.Items {
			if !eventRecent(e, cutoff) {
				continue
			}
			if e.Type == corev1.EventTypeWarning {
				h.WarningEvents++
				if isErrorish(e) {
					h.ErrorEvents++
				}
			}
		}
	}
	return h, nil
}

func nodeReady(n corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func oomKilled(p corev1.Pod) bool {
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
			return true
		}
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			return true
		}
	}
	return false
}

func restartCount(p corev1.Pod) int {
	n := 0
	for _, cs := range p.Status.ContainerStatuses {
		n += int(cs.RestartCount)
	}
	return n
}

func isErrorish(e corev1.Event) bool {
	r := strings.ToLower(e.Reason + " " + e.Message)
	for _, k := range []string{"oom", "failed", "unhealthy", "notready", "kill", "crash", "evict"} {
		if strings.Contains(r, k) {
			return true
		}
	}
	return false
}

func eventRecent(e corev1.Event, since time.Time) bool {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time.After(since)
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time.After(since)
	}
	return true
}
