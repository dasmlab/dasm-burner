package ovndiag

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// Log class tokens used to normalize noisy OVN/OVS messages into findings.
var logClasses = []struct {
	Name    string
	Needles []string
}{
	{"ERROR", []string{"error", " err=", "fatal"}},
	{"WARN", []string{"warning", " warn "}},
	{"CONNECTION", []string{"connection refused", "connection reset", "broken pipe", "eof"}},
	{"TIMEOUT", []string{"timeout", "deadline exceeded", "i/o timeout"}},
	{"RAFT", []string{"raft", "election", "leader"}},
	{"DATABASE", []string{"nbdb", "sbdb", "ovsdb", "transaction"}},
	{"NETLINK", []string{"netlink"}},
	{"IPTABLES", []string{"iptables", "nft"}},
	{"FLOW", []string{"flow", "ofctl"}},
	{"GATEWAY", []string{"gateway", "br-ex"}},
	{"DROP", []string{"packet drop", "dropped packet", "packets dropped", "conntrack table full", "openflow drop"}},
	{"OVS", []string{"ovs-vswitchd", "ovsdb-server"}},
	{"OVN", []string{"ovn-controller", "ovnkube"}},
	{"KERNEL", []string{"kernel", "oops", "soft lockup"}},
}

type logHit struct {
	Class  string
	Sample string
	Count  int
	First  time.Time
	Last   time.Time
	Pod    string
	Node   string
}

// ScanOVNLogs pulls a short tail from ovnkube-node pods and buckets ERROR/WARN-ish lines.
// Prefer pods already flagged; otherwise sample up to maxPods workers.
func ScanOVNLogs(ctx context.Context, cs kubernetes.Interface, pods []corev1.Pod, hotNodes map[string]bool, maxPods int) []Finding {
	if maxPods <= 0 {
		maxPods = 6
	}
	now := time.Now()
	var targets []corev1.Pod
	for _, p := range pods {
		if !containsOVNKubeNode(p.Name) {
			continue
		}
		if hotNodes[p.Spec.NodeName] {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		for _, p := range pods {
			if containsOVNKubeNode(p.Name) {
				targets = append(targets, p)
				if len(targets) >= maxPods {
					break
				}
			}
		}
	} else if len(targets) > maxPods {
		targets = targets[:maxPods]
	}

	hits := map[string]*logHit{}
	for _, p := range targets {
		container := pickLogContainer(p)
		if container == "" {
			continue
		}
		tail := int64(200)
		req := cs.CoreV1().Pods(ovnNS).GetLogs(p.Name, &corev1.PodLogOptions{
			Container:  container,
			TailLines:  &tail,
			Timestamps: true,
		})
		stream, err := req.Stream(ctx)
		if err != nil {
			continue
		}
		ingestLogStream(stream, p, hits, now)
		_ = stream.Close()
	}

	var out []Finding
	for _, h := range hits {
		if h.Count < 1 {
			continue
		}
		sev := SevNotice
		if h.Class == "ERROR" || h.Class == "TIMEOUT" || h.Class == "CONNECTION" || h.Class == "DROP" {
			sev = SevWarning
		}
		if h.Count >= 20 && (h.Class == "ERROR" || h.Class == "TIMEOUT" || h.Class == "DROP") {
			sev = SevError
		}
		rule := RuleLogAnomaly
		cat := CatLog
		if h.Class == "ERROR" && h.Count >= 10 {
			rule = RuleErrorRateAccel
		}
		if h.Class == "DROP" {
			rule = RulePacketDrop
			cat = CatDataplane
		}
		durMin := h.Last.Sub(h.First).Minutes()
		if durMin < 0.05 {
			durMin = 0.05
		}
		rate := float64(h.Count) / durMin
		out = append(out, Finding{
			ID:        fmt.Sprintf("%s-%s-%s-%d", rule, h.Node, h.Class, h.Last.Unix()),
			RuleID:    rule,
			Severity:  sev,
			Category:  cat,
			Node:      h.Node,
			Component: h.Pod,
			FirstSeen: h.First,
			LastSeen:  h.Last,
			Count:     h.Count,
			Summary:   fmt.Sprintf("OVN log class %s ×%d (%.1f/min) on %s", h.Class, h.Count, rate, h.Node),
			Evidence: []Evidence{
				{Label: "class", Current: h.Class},
				{Label: "rate_per_min", Current: fmt.Sprintf("%.1f", rate)},
				{Label: "sample", Current: truncate(h.Sample, 200)},
			},
			Why: "Normalized OVN/OVS log scanner (not raw dump).",
		})
	}
	return out
}

func pickLogContainer(p corev1.Pod) string {
	prefer := []string{"ovnkube-controller", "ovn-controller", "northd", "sbdb", "nbdb"}
	have := map[string]bool{}
	for _, c := range p.Spec.Containers {
		have[c.Name] = true
	}
	for _, name := range prefer {
		if have[name] {
			return name
		}
	}
	if len(p.Spec.Containers) > 0 {
		return p.Spec.Containers[0].Name
	}
	return ""
}

func ingestLogStream(r io.Reader, p corev1.Pod, hits map[string]*logHit, now time.Time) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		low := strings.ToLower(line)
		cls := classifyLog(low)
		if cls == "" {
			continue
		}
		key := p.Spec.NodeName + "|" + cls
		h := hits[key]
		if h == nil {
			h = &logHit{Class: cls, Pod: p.Name, Node: p.Spec.NodeName, First: now, Last: now, Sample: truncate(line, 200)}
			hits[key] = h
		}
		h.Count++
		h.Last = now
		if h.Sample == "" {
			h.Sample = truncate(line, 200)
		}
	}
}

func classifyLog(low string) string {
	// Prefer specific classes before generic OVN/OVS.
	order := []string{"KERNEL", "RAFT", "DATABASE", "CONNECTION", "TIMEOUT", "DROP", "NETLINK", "IPTABLES", "FLOW", "GATEWAY", "ERROR", "WARN", "OVS", "OVN"}
	byName := map[string][]string{}
	for _, c := range logClasses {
		byName[c.Name] = c.Needles
	}
	for _, name := range order {
		for _, n := range byName[name] {
			if strings.Contains(low, n) {
				// Skip noisy info-level hits unless error/warn/fail also present.
				if name == "OVN" || name == "OVS" || name == "IPTABLES" || name == "FLOW" || name == "GATEWAY" {
					if !(strings.Contains(low, "error") || strings.Contains(low, "warn") ||
						strings.Contains(low, "fail") || strings.Contains(low, "drop")) {
						continue
					}
				}
				return name
			}
		}
	}
	return ""
}
