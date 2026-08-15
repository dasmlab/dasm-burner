package etcddiag

import (
	"context"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var podMetricsGVR = schema.GroupVersionResource{
	Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods",
}

func containerRSSMi(ctx context.Context, dyn dynamic.Interface, ns, pod, container string) (float64, bool) {
	if dyn == nil || pod == "" {
		return 0, false
	}
	u, err := dyn.Resource(podMetricsGVR).Namespace(ns).Get(ctx, pod, metav1.GetOptions{})
	if err != nil {
		return 0, false
	}
	raw, ok := u.Object["containers"].([]any)
	if !ok {
		return 0, false
	}
	for _, c := range raw {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if container != "" && name != container {
			continue
		}
		usage, _ := m["usage"].(map[string]any)
		mem, _ := usage["memory"].(string)
		return parseMemoryMiB(mem), true
	}
	return 0, false
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
	default:
		n, _ := strconv.ParseFloat(s, 64)
		return n / (1024 * 1024)
	}
	n, _ := strconv.ParseFloat(s, 64)
	return n * mult
}
