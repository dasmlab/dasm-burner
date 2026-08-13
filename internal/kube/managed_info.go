package kube

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dasmlab/dasm-burner/internal/topology"
)

// ManagedNSInfo is one managed namespace with run/config labels (for template matching).
type ManagedNSInfo struct {
	Name   string `json:"name"`
	RunID  string `json:"runId,omitempty"`
	Config string `json:"config,omitempty"` // dasm-burner.dasmlab.org/config (= template name)
}

// ListManagedNamespaceInfo returns managed namespaces with run + config labels.
func (l *Live) ListManagedNamespaceInfo(ctx context.Context, runID string) ([]ManagedNSInfo, error) {
	list, err := l.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: topology.Selector(runID)})
	if err != nil {
		return nil, err
	}
	out := make([]ManagedNSInfo, 0, len(list.Items))
	for _, ns := range list.Items {
		info := ManagedNSInfo{Name: ns.Name}
		if ns.Labels != nil {
			info.RunID = ns.Labels[topology.LabelRun]
			info.Config = ns.Labels[topology.LabelConfig]
		}
		if info.RunID == "" {
			info.RunID = runIDFromName(ns.Name)
		}
		out = append(out, info)
	}
	return out, nil
}

func runIDFromName(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) >= 3 && parts[0] == "kb" {
		return parts[1]
	}
	return ""
}
