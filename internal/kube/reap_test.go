package kube

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

func TestReapLabeledDeletesOrphansWithoutNamespace(t *testing.T) {
	labels := map[string]string{topology.LabelManaged: "true"}
	cs := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "kb-gone", Labels: labels}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "kb-gone", Labels: labels}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "kb-gone", Labels: labels}},
	)
	n, err := ReapLabeled(context.Background(), cs, nil, "", false, 2, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("deleted %d want 3", n)
	}
	if list, _ := cs.CoreV1().Pods(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{LabelSelector: topology.Selector("")}); len(list.Items) != 0 {
		t.Fatalf("pods left %d", len(list.Items))
	}
	if list, _ := cs.AppsV1().Deployments(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{LabelSelector: topology.Selector("")}); len(list.Items) != 0 {
		t.Fatalf("deploys left %d", len(list.Items))
	}
}
