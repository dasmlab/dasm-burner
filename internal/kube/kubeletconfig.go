package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	WorkerKubeletConfigName = "dasm-burner-worker-maxpods"
	WorkerMCPName           = "worker"
	MinMaxPods              = 110
	MaxMaxPods              = 2000
)

var (
	kubeletConfigGVR = schema.GroupVersionResource{
		Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "kubeletconfigs",
	}
	mcpGVR = schema.GroupVersionResource{
		Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools",
	}
)

// ValidateMaxPods bounds the kubelet maxPods value.
func ValidateMaxPods(n int) error {
	if n < MinMaxPods || n > MaxMaxPods {
		return fmt.Errorf("maxPods must be between %d and %d", MinMaxPods, MaxMaxPods)
	}
	return nil
}

// WorkerKubeletConfigObject is the KubeletConfig dasm-burner applies to the worker MCP.
func WorkerKubeletConfigObject(maxPods int) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "machineconfiguration.openshift.io/v1",
		"kind":       "KubeletConfig",
		"metadata": map[string]any{
			"name": WorkerKubeletConfigName,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "dasm-burner",
			},
		},
		"spec": map[string]any{
			"machineConfigPoolSelector": map[string]any{
				"matchLabels": map[string]any{
					"pools.operator.machineconfiguration.openshift.io/worker": "",
				},
			},
			"kubeletConfig": map[string]any{
				"maxPods": int64(maxPods),
			},
		},
	}}
}

// EnsureWorkerMCPSerial sets worker MachineConfigPool maxUnavailable=1 (one node at a time).
func EnsureWorkerMCPSerial(ctx context.Context, dyn dynamic.Interface) error {
	if dyn == nil {
		return fmt.Errorf("no dynamic client")
	}
	mcp, err := dyn.Resource(mcpGVR).Get(ctx, WorkerMCPName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get MachineConfigPool %s: %w", WorkerMCPName, err)
	}
	if err := unstructured.SetNestedField(mcp.Object, int64(1), "spec", "maxUnavailable"); err != nil {
		return err
	}
	_, err = dyn.Resource(mcpGVR).Update(ctx, mcp, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("set %s maxUnavailable=1: %w", WorkerMCPName, err)
	}
	return nil
}

// ApplyWorkerMaxPods creates or updates the worker KubeletConfig.
func ApplyWorkerMaxPods(ctx context.Context, dyn dynamic.Interface, maxPods int) error {
	if err := ValidateMaxPods(maxPods); err != nil {
		return err
	}
	if dyn == nil {
		return fmt.Errorf("no dynamic client")
	}
	want := WorkerKubeletConfigObject(maxPods)
	got, err := dyn.Resource(kubeletConfigGVR).Get(ctx, WorkerKubeletConfigName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = dyn.Resource(kubeletConfigGVR).Create(ctx, want, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create KubeletConfig: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get KubeletConfig: %w", err)
	}
	if err := unstructured.SetNestedField(got.Object, int64(maxPods), "spec", "kubeletConfig", "maxPods"); err != nil {
		return err
	}
	_, err = dyn.Resource(kubeletConfigGVR).Update(ctx, got, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update KubeletConfig: %w", err)
	}
	return nil
}

// MCPRoll is a compact MachineConfigPool status snapshot.
type MCPRoll struct {
	Name            string `json:"name"`
	Updated         bool   `json:"updated"`
	Updating        bool   `json:"updating"`
	Degraded        bool   `json:"degraded"`
	MachineCount    int64  `json:"machineCount"`
	ReadyCount      int64  `json:"readyMachineCount"`
	UpdatedCount    int64  `json:"updatedMachineCount"`
	DegradedCount   int64  `json:"degradedMachineCount"`
	MaxUnavailable  any    `json:"maxUnavailable,omitempty"`
	Summary         string `json:"summary"`
}

func ReadMCPRoll(ctx context.Context, dyn dynamic.Interface, name string) (MCPRoll, error) {
	out := MCPRoll{Name: name}
	if dyn == nil {
		return out, fmt.Errorf("no dynamic client")
	}
	mcp, err := dyn.Resource(mcpGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return out, err
	}
	st, _, _ := unstructured.NestedMap(mcp.Object, "status")
	out.Updated = asBool(st["updated"])
	out.Updating = asBool(st["updating"])
	out.Degraded = asBool(st["degraded"])
	out.MachineCount = asInt64(st["machineCount"])
	out.ReadyCount = asInt64(st["readyMachineCount"])
	out.UpdatedCount = asInt64(st["updatedMachineCount"])
	out.DegradedCount = asInt64(st["degradedMachineCount"])
	out.MaxUnavailable, _, _ = unstructured.NestedFieldNoCopy(mcp.Object, "spec", "maxUnavailable")
	out.Summary = fmt.Sprintf("mcp/%s updated=%v updating=%v degraded=%v machines %d ready %d rendered %d",
		name, out.Updated, out.Updating, out.Degraded, out.MachineCount, out.ReadyCount, out.UpdatedCount)
	return out, nil
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
