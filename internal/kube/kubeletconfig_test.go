package kube

import "testing"

func TestValidateMaxPods(t *testing.T) {
	t.Parallel()
	if err := ValidateMaxPods(500); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMaxPods(50); err == nil {
		t.Fatal("expected low bound")
	}
	if err := ValidateMaxPods(9000); err == nil {
		t.Fatal("expected high bound")
	}
}

func TestWorkerKubeletConfigObject(t *testing.T) {
	t.Parallel()
	u := WorkerKubeletConfigObject(500)
	if u.GetName() != WorkerKubeletConfigName {
		t.Fatalf("name %s", u.GetName())
	}
	n, ok, err := unstructuredInt(u.Object, "spec", "kubeletConfig", "maxPods")
	if err != nil || !ok || n != 500 {
		t.Fatalf("maxPods=%v ok=%v err=%v", n, ok, err)
	}
	sel, _, _ := nestedString(u.Object, "spec", "machineConfigPoolSelector", "matchLabels")
	if sel["pools.operator.machineconfiguration.openshift.io/worker"] != "" {
		// empty string value is present
	}
	labels, _, _ := nestedString(u.Object, "spec", "machineConfigPoolSelector", "matchLabels")
	if _, ok := labels["pools.operator.machineconfiguration.openshift.io/worker"]; !ok {
		t.Fatal("missing worker pool selector")
	}
}

func TestClassifyMaxPodsRollout(t *testing.T) {
	t.Parallel()
	yes, _ := ClassifyMaxPodsRollout(WorkerMaxPodsStatus{
		Configured: true, Desired: 1000, MatchingNodes: 15, WorkerNodes: 15,
		ObservedMin: 1000, ObservedMax: 1000,
		MCP: MCPRoll{MachineCount: 15, UpdatedCount: 15},
	})
	if yes != "yes" {
		t.Fatalf("all match = %s", yes)
	}
	partial, _ := ClassifyMaxPodsRollout(WorkerMaxPodsStatus{
		Configured: true, Desired: 1000, MatchingNodes: 4, WorkerNodes: 15,
		ObservedMin: 250, ObservedMax: 1000,
		MCP: MCPRoll{MachineCount: 15, UpdatedCount: 15},
	})
	if partial != "partial" {
		t.Fatalf("mixed = %s", partial)
	}
	rolling, reason := ClassifyMaxPodsRollout(WorkerMaxPodsStatus{
		Configured: true, Desired: 1000, MatchingNodes: 15, WorkerNodes: 15,
		MCP: MCPRoll{Updating: true, MachineCount: 15, UpdatedCount: 3},
	})
	if rolling != "partial" {
		t.Fatalf("updating = %s (%s)", rolling, reason)
	}
	none, _ := ClassifyMaxPodsRollout(WorkerMaxPodsStatus{
		Configured: false, WorkerNodes: 15, ObservedMin: 250, ObservedMax: 250,
	})
	if none != "no" {
		t.Fatalf("unset = %s", none)
	}
}

func unstructuredInt(obj map[string]any, fields ...string) (int64, bool, error) {
	cur := any(obj)
	for _, f := range fields[:len(fields)-1] {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false, nil
		}
		cur = m[f]
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return 0, false, nil
	}
	v, ok := m[fields[len(fields)-1]]
	if !ok {
		return 0, false, nil
	}
	switch n := v.(type) {
	case int64:
		return n, true, nil
	case int:
		return int64(n), true, nil
	default:
		return 0, false, nil
	}
}

func nestedString(obj map[string]any, fields ...string) (map[string]any, bool, error) {
	cur := any(obj)
	for _, f := range fields {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		cur = m[f]
	}
	m, ok := cur.(map[string]any)
	return m, ok, nil
}
