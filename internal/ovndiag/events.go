package ovndiag

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type eventBucket struct {
	Reason   string
	Message  string
	Count    int
	First    time.Time
	Last     time.Time
	Nodes    map[string]struct{}
	Objects  map[string]struct{}
	Errorish bool
}

// CollectOVNEvents aggregates Warning events in the OVN namespace over the window.
func CollectOVNEvents(ctx context.Context, cs kubernetes.Interface, window time.Duration) ([]Finding, []TimelineEvent) {
	cutoff := time.Now().Add(-window)
	evs, err := cs.CoreV1().Events(ovnNS).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, nil
	}
	buckets := map[string]*eventBucket{}
	now := time.Now()
	for _, e := range evs.Items {
		if e.Type != corev1.EventTypeWarning {
			continue
		}
		ts := eventTime(e)
		if ts.Before(cutoff) {
			continue
		}
		key := e.Reason + "|" + normalizeMsg(e.Message)
		b := buckets[key]
		if b == nil {
			b = &eventBucket{
				Reason:   e.Reason,
				Message:  truncate(e.Message, 160),
				First:    ts,
				Last:     ts,
				Nodes:    map[string]struct{}{},
				Objects:  map[string]struct{}{},
				Errorish: isErrorishEvent(e),
			}
			buckets[key] = b
		}
		n := e.Count
		if n < 1 {
			n = 1
		}
		b.Count += int(n)
		if ts.Before(b.First) {
			b.First = ts
		}
		if ts.After(b.Last) {
			b.Last = ts
		}
		obj := e.InvolvedObject.Name
		b.Objects[obj] = struct{}{}
		if node := nodeFromOVNPodName(obj); node != "" {
			b.Nodes[node] = struct{}{}
		}
	}

	var findings []Finding
	var tl []TimelineEvent
	for _, b := range buckets {
		sev := SevWarning
		if b.Errorish || strings.Contains(strings.ToLower(b.Reason), "fail") ||
			strings.Contains(strings.ToLower(b.Reason), "unhealthy") {
			sev = SevError
		}
		nodes := keys(b.Nodes)
		node := ""
		if len(nodes) == 1 {
			node = nodes[0]
		}
		rate := 0.0
		durMin := b.Last.Sub(b.First).Minutes()
		if durMin < 0.05 {
			durMin = 0.05
		}
		rate = float64(b.Count) / durMin
		f := Finding{
			ID:        fmt.Sprintf("%s-%s-%d", RuleEventBurst, b.Reason, b.Last.Unix()),
			RuleID:    RuleEventBurst,
			Severity:  sev,
			Category:  CatOVNKube,
			Node:      node,
			Component: "events",
			FirstSeen: b.First,
			LastSeen:  b.Last,
			Count:     b.Count,
			Summary:   fmt.Sprintf("OVN Warning events: %s ×%d (%.1f/min)", b.Reason, b.Count, rate),
			Evidence: []Evidence{
				{Label: "reason", Current: b.Reason},
				{Label: "sample", Current: b.Message},
				{Label: "rate_per_min", Current: fmt.Sprintf("%.1f", rate)},
				{Label: "objects", Current: strings.Join(keys(b.Objects), ",")},
			},
			Why: fmt.Sprintf("Cluster Warning events in %s: %s", ovnNS, b.Reason),
		}
		findings = append(findings, f)
		tl = append(tl, TimelineEvent{
			At: now, Kind: "finding", Summary: f.Summary, Node: node, Finding: f.ID, Severity: sev,
		})
	}
	return findings, tl
}

func eventTime(e corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	if !e.FirstTimestamp.IsZero() {
		return e.FirstTimestamp.Time
	}
	return time.Now()
}

func isErrorishEvent(e corev1.Event) bool {
	blob := strings.ToLower(e.Reason + " " + e.Message)
	for _, k := range []string{"error", "fail", "oom", "crash", "timeout", "refused", "backoff", "kill"} {
		if strings.Contains(blob, k) {
			return true
		}
	}
	return false
}

func normalizeMsg(m string) string {
	m = strings.TrimSpace(m)
	if len(m) > 80 {
		m = m[:80]
	}
	return m
}

func nodeFromOVNPodName(pod string) string {
	// Cannot map reliably without pod list; leave empty for multi-object buckets.
	_ = pod
	return ""
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
