package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCountNodeRolesMultiLabel(t *testing.T) {
	t.Parallel()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "cp", Labels: map[string]string{
			"node-role.kubernetes.io/control-plane": "",
			"node-role.kubernetes.io/master":        "",
		}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "infra1", Labels: map[string]string{
			"node-role.kubernetes.io/infra":  "",
			"node-role.kubernetes.io/worker": "",
		}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "w1", Labels: map[string]string{
			"node-role.kubernetes.io/worker": "",
		}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
	}
	roles := countNodeRoles(nodes)
	by := map[string]NodeRoleCount{}
	for _, r := range roles {
		by[r.Role] = r
	}
	if by["control-plane"].Total != 1 || by["master"].Total != 1 {
		t.Fatalf("cp/master: %+v", by)
	}
	if by["infra"].Total != 1 || by["infra"].NotReady != 1 || by["infra"].Nodes[0] != "infra1" {
		t.Fatalf("infra: %+v", by["infra"])
	}
	if by["worker"].Total != 2 || by["worker"].Ready != 1 || by["worker"].NotReady != 1 {
		t.Fatalf("worker: %+v", by["worker"])
	}
}
