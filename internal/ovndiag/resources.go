package ovndiag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var podMetricsGVR = schema.GroupVersionResource{
	Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods",
}

// ResourceSample is one container's CPU/memory from metrics.k8s.io (when available).
type ResourceSample struct {
	Container string  `json:"container"`
	CPUCores  float64 `json:"cpuCores"`
	MemoryMiB float64 `json:"memoryMiB"`
}

// CollectPodResources returns per-container usage for an OVN pod via metrics.k8s.io.
// Missing metrics API → empty slice (capability gated).
func CollectPodResources(ctx context.Context, dyn dynamic.Interface, podName string) ([]ResourceSample, error) {
	if dyn == nil || podName == "" {
		return nil, nil
	}
	u, err := dyn.Resource(podMetricsGVR).Namespace(ovnNS).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	containers, ok, _ := unstructuredSlice(u.Object, "containers")
	if !ok {
		return nil, nil
	}
	var out []ResourceSample
	for _, c := range containers {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		usage, _ := m["usage"].(map[string]any)
		cpuStr, _ := usage["cpu"].(string)
		memStr, _ := usage["memory"].(string)
		out = append(out, ResourceSample{
			Container: name,
			CPUCores:  parseCPUCores(cpuStr),
			MemoryMiB: parseMemoryMiB(memStr),
		})
	}
	return out, nil
}

func MetricsAPIAvailable(ctx context.Context, dyn dynamic.Interface) bool {
	if dyn == nil {
		return false
	}
	_, err := dyn.Resource(podMetricsGVR).Namespace(ovnNS).List(ctx, metav1.ListOptions{Limit: 1})
	return err == nil
}

func evaluateResources(node string, samples []ResourceSample, baseline *Baseline, now time.Time, batchID int) []Finding {
	var out []Finding
	var cpuSum, memSum float64
	for _, s := range samples {
		cpuSum += s.CPUCores
		memSum += s.MemoryMiB
		// Hot containers: ovn-controller / northd / sbdb often matter most
		interesting := s.Container == "ovn-controller" || s.Container == "northd" ||
			s.Container == "sbdb" || s.Container == "nbdb" || s.Container == "ovnkube-controller"
		if !interesting {
			continue
		}
		if baseCPU, ok := baseline.CPUWatermark(node, s.Container); ok && baseCPU > 0 {
			// anomaly if > 2.5x baseline and absolute > 0.15 cores
			if s.CPUCores > baseCPU*2.5 && s.CPUCores > 0.15 {
				out = append(out, Finding{
					ID:     fmt.Sprintf("%s-%s-%s-%d", RuleResourceCPU, node, s.Container, now.Unix()),
					RuleID: RuleResourceCPU, Severity: SevWarning, Category: CatResource,
					Node: node, Component: s.Container, FirstSeen: now, LastSeen: now, Count: 1,
					Summary: fmt.Sprintf("%s CPU %.2fc vs baseline %.2fc", s.Container, s.CPUCores, baseCPU),
					Evidence: []Evidence{{
						Label: "cpu_cores", Baseline: fmt.Sprintf("%.3f", baseCPU),
						Current: fmt.Sprintf("%.3f", s.CPUCores),
						Delta:   fmt.Sprintf("+%.0f%%", (s.CPUCores/baseCPU-1)*100),
					}},
					BatchID: batchID,
					Why:     "Container CPU rose materially above the pre-load baseline (change, not absolute threshold).",
				})
			}
		}
		if baseMem, ok := baseline.MemWatermark(node, s.Container); ok && baseMem > 0 {
			if s.MemoryMiB > baseMem*1.75 && s.MemoryMiB-baseMem > 100 {
				out = append(out, Finding{
					ID:     fmt.Sprintf("%s-%s-%s-%d", RuleResourceMem, node, s.Container, now.Unix()),
					RuleID: RuleResourceMem, Severity: SevWarning, Category: CatResource,
					Node: node, Component: s.Container, FirstSeen: now, LastSeen: now, Count: 1,
					Summary: fmt.Sprintf("%s memory %.0fMi vs baseline %.0fMi", s.Container, s.MemoryMiB, baseMem),
					Evidence: []Evidence{{
						Label: "memory_mib", Baseline: fmt.Sprintf("%.0f", baseMem),
						Current: fmt.Sprintf("%.0f", s.MemoryMiB),
						Delta:   fmt.Sprintf("+%.0f MiB", s.MemoryMiB-baseMem),
					}},
					BatchID: batchID,
					Why:     "Container memory working set grew vs baseline — watch for acceleration across batches.",
				})
			}
		}
	}
	_ = cpuSum
	_ = memSum
	return out
}

func unstructuredSlice(obj map[string]any, key string) ([]any, bool, error) {
	v, ok := obj[key]
	if !ok {
		return nil, false, nil
	}
	arr, ok := v.([]any)
	return arr, ok, nil
}

func parseCPUCores(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "n") {
		n, _ := strconv.ParseFloat(strings.TrimSuffix(s, "n"), 64)
		return n / 1e9
	}
	if strings.HasSuffix(s, "u") {
		n, _ := strconv.ParseFloat(strings.TrimSuffix(s, "u"), 64)
		return n / 1e6
	}
	if strings.HasSuffix(s, "m") {
		n, _ := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		return n / 1000
	}
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func parseMemoryMiB(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "Ki"):
		mult = 1.0 / 1024
		s = strings.TrimSuffix(s, "Ki")
	case strings.HasSuffix(s, "Mi"):
		mult = 1
		s = strings.TrimSuffix(s, "Mi")
	case strings.HasSuffix(s, "Gi"):
		mult = 1024
		s = strings.TrimSuffix(s, "Gi")
	case strings.HasSuffix(s, "Ti"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "Ti")
	case strings.HasSuffix(s, "k"):
		mult = 1000.0 / (1024 * 1024)
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "M"):
		mult = 1000.0 * 1000 / (1024 * 1024)
		s = strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "G"):
		mult = 1e9 / (1024 * 1024)
		s = strings.TrimSuffix(s, "G")
	default:
		// bytes
		n, _ := strconv.ParseFloat(s, 64)
		return n / (1024 * 1024)
	}
	n, _ := strconv.ParseFloat(s, 64)
	return n * mult
}
