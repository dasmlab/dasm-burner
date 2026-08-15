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

// OVNPodDetail is one openshift-ovn-kubernetes pod (typically ovnkube-node / controller).
type OVNPodDetail struct {
	Name          string `json:"name"`
	Node          string `json:"node,omitempty"`
	Ready         bool   `json:"ready"`
	Restarts      int    `json:"restarts"`
	RestartsDelta int    `json:"restartsDelta,omitempty"`
	Phase         string `json:"phase,omitempty"`
}

// Health is a cluster snapshot used for abort gates and the OVN report.
type Health struct {
	SampledAt     time.Time `json:"sampledAt"`
	NodesReady    int       `json:"nodesReady"`
	NodesNotReady int       `json:"nodesNotReady"`
	NotReadyNodes []string  `json:"notReadyNodes,omitempty"`

	MastersReady         int      `json:"mastersReady"`
	MastersNotReady      int      `json:"mastersNotReady"`
	MasterNotReadyNodes  []string `json:"masterNotReadyNodes,omitempty"`
	MastersMemoryPressure int     `json:"mastersMemoryPressure,omitempty"`

	EtcdPods     int `json:"etcdPods,omitempty"`
	EtcdReady    int `json:"etcdReady,omitempty"`
	EtcdRestarts int `json:"etcdRestarts,omitempty"`

	ManagedPods  int `json:"managedPods"`
	ManagedReady int `json:"managedReadyPods"`
	FailedPods   int `json:"failedPods"`
	OOMKilled    int `json:"oomKilled"`

	OVNPods          int            `json:"ovnPods"`
	OVNReady         int            `json:"ovnReady"`
	OVNRestarts      int            `json:"ovnRestarts"`
	OVNRestartsDelta int            `json:"ovnRestartsDelta,omitempty"`
	OVNDetail        []OVNPodDetail `json:"ovnDetail,omitempty"`

	WarningEvents int `json:"warningEvents"`
	ErrorEvents   int `json:"errorEvents"`

	NodeRoles []NodeRoleCount `json:"nodeRoles,omitempty"`
}

// NodeRoleCount is Ready/NotReady for one node-role.kubernetes.io/* label.
// Multi-role nodes count in each matching bucket.
type NodeRoleCount struct {
	Role     string   `json:"role"`
	Ready    int      `json:"ready"`
	NotReady int      `json:"notReady"`
	Total    int      `json:"total"`
	Nodes    []string `json:"nodes,omitempty"` // NotReady node names
}

func (l *Live) ClusterHealth(ctx context.Context, runID string) (Health, error) {
	h := Health{SampledAt: time.Now()}

	nodes, err := l.cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return h, err
	}
	for _, n := range nodes.Items {
		ready := nodeReady(n)
		if ready {
			h.NodesReady++
		} else {
			h.NodesNotReady++
			h.NotReadyNodes = append(h.NotReadyNodes, n.Name)
		}
		if isControlPlaneNode(n) {
			if ready {
				h.MastersReady++
			} else {
				h.MastersNotReady++
				h.MasterNotReadyNodes = append(h.MasterNotReadyNodes, n.Name)
			}
			if nodeConditionTrue(n, corev1.NodeMemoryPressure) {
				h.MastersMemoryPressure++
			}
		}
	}
	h.NodeRoles = countNodeRoles(nodes.Items)

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
			ready := podReady(p)
			if ready {
				h.OVNReady++
			}
			rc := restartCount(p)
			h.OVNRestarts += rc
			h.OVNDetail = append(h.OVNDetail, OVNPodDetail{
				Name:     p.Name,
				Node:     p.Spec.NodeName,
				Ready:    ready,
				Restarts: rc,
				Phase:    string(p.Status.Phase),
			})
		}
	}

	if etcd, err := l.cs.CoreV1().Pods("openshift-etcd").List(ctx, metav1.ListOptions{
		LabelSelector: "app=etcd",
	}); err == nil {
		for _, p := range etcd.Items {
			h.EtcdPods++
			if podReady(p) {
				h.EtcdReady++
			}
			h.EtcdRestarts += restartCount(p)
		}
	} else if etcd, err := l.cs.CoreV1().Pods("openshift-etcd").List(ctx, metav1.ListOptions{}); err == nil {
		for _, p := range etcd.Items {
			if !strings.HasPrefix(p.Name, "etcd-") {
				continue
			}
			h.EtcdPods++
			if podReady(p) {
				h.EtcdReady++
			}
			h.EtcdRestarts += restartCount(p)
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

// ApplyOVNRestartDeltas sets RestartsDelta on close relative to open watermarks (by pod name).
func ApplyOVNRestartDeltas(open, close Health) Health {
	base := map[string]int{}
	for _, p := range open.OVNDetail {
		base[p.Name] = p.Restarts
	}
	var deltaSum int
	out := close
	detail := make([]OVNPodDetail, len(close.OVNDetail))
	for i, p := range close.OVNDetail {
		d := p
		if prev, ok := base[p.Name]; ok {
			d.RestartsDelta = p.Restarts - prev
			if d.RestartsDelta < 0 {
				d.RestartsDelta = 0
			}
		} else {
			d.RestartsDelta = 0 // new pod during run — don't blame lifetime count
		}
		deltaSum += d.RestartsDelta
		detail[i] = d
	}
	out.OVNDetail = detail
	out.OVNRestartsDelta = deltaSum
	return out
}

func nodeReady(n corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isControlPlaneNode(n corev1.Node) bool {
	if n.Labels == nil {
		return false
	}
	if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
		return true
	}
	if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
		return true
	}
	return false
}

func nodeConditionTrue(n corev1.Node, t corev1.NodeConditionType) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == t {
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

var nodeRoleOrder = []string{"control-plane", "master", "worker", "infra", "other"}

func countNodeRoles(nodes []corev1.Node) []NodeRoleCount {
	byRole := map[string]*NodeRoleCount{}
	ensure := func(role string) *NodeRoleCount {
		if c, ok := byRole[role]; ok {
			return c
		}
		c := &NodeRoleCount{Role: role}
		byRole[role] = c
		return c
	}
	for _, n := range nodes {
		roles := nodeRolesFromLabels(n.Labels)
		ready := nodeReady(n)
		for _, role := range roles {
			c := ensure(role)
			c.Total++
			if ready {
				c.Ready++
			} else {
				c.NotReady++
				c.Nodes = append(c.Nodes, n.Name)
			}
		}
	}
	var out []NodeRoleCount
	for _, role := range nodeRoleOrder {
		if c, ok := byRole[role]; ok {
			out = append(out, *c)
			delete(byRole, role)
		}
	}
	for _, c := range byRole {
		out = append(out, *c)
	}
	return out
}

func nodeRolesFromLabels(labels map[string]string) []string {
	if labels == nil {
		return []string{"other"}
	}
	var roles []string
	for _, role := range []string{"control-plane", "master", "worker", "infra"} {
		if _, ok := labels["node-role.kubernetes.io/"+role]; ok {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return []string{"other"}
	}
	return roles
}
