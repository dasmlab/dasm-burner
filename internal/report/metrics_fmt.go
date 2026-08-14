package report

import (
	"fmt"
	"strings"
	"unicode"
)

// HumanLabel turns apiLatencyP99 into "API latency p99".
func HumanLabel(metric string) string {
	s := strings.TrimSpace(metric)
	if s == "" {
		return "metric"
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rune(s[i-1])) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	repl := []struct{ old, neu string }{
		{"api ", "API "},
		{"Api ", "API "},
		{"etcd ", "etcd "},
		{"ovn ", "OVN "},
		{"Ovn ", "OVN "},
		{"cpu ", "CPU "},
		{"p99", "p99"},
		{"P99", "p99"},
	}
	for _, r := range repl {
		out = strings.ReplaceAll(out, r.old, r.neu)
	}
	if len(out) > 0 {
		out = strings.ToUpper(out[:1]) + out[1:]
	}
	return out
}

// HumanValue formats a Prometheus sample for operators (units, not scientific notation).
func HumanValue(metric string, v float64) string {
	key := strings.ToLower(metric)
	switch {
	case strings.Contains(key, "memory") || strings.Contains(key, "bytes"):
		return formatBytes(v)
	case strings.Contains(key, "cpu"):
		return formatCPU(v)
	case strings.Contains(key, "latency"), strings.Contains(key, "wal"), strings.Contains(key, "fsync"), strings.Contains(key, "duration"):
		return formatSeconds(v)
	case strings.Contains(key, "errorrate"):
		return formatRate(v, "err/s")
	case strings.Contains(key, "requestrate"):
		return formatRate(v, "req/s")
	case strings.Contains(key, "ready"):
		if v >= 0 && v <= 1.05 {
			return fmt.Sprintf("%.1f%%", v*100)
		}
		return formatPlain(v)
	default:
		return formatPlain(v)
	}
}

func formatBytes(v float64) string {
	if v < 0 {
		v = -v
	}
	switch {
	case v >= 1024*1024*1024:
		return fmt.Sprintf("%.2f GiB", v/(1024*1024*1024))
	case v >= 1024*1024:
		return fmt.Sprintf("%.0f MiB", v/(1024*1024))
	case v >= 1024:
		return fmt.Sprintf("%.1f KiB", v/1024)
	default:
		return fmt.Sprintf("%.0f B", v)
	}
}

func formatCPU(v float64) string {
	if v < 0.01 {
		return fmt.Sprintf("%.2f mCPU", v*1000)
	}
	if v < 1 {
		return fmt.Sprintf("%.3f CPU", v)
	}
	return fmt.Sprintf("%.2f CPU", v)
}

func formatSeconds(v float64) string {
	if v < 1 {
		return fmt.Sprintf("%.1f ms", v*1000)
	}
	return fmt.Sprintf("%.2f s", v)
}

func formatRate(v float64, unit string) string {
	if v == 0 {
		return "0 " + unit
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f %s", v, unit)
	}
	return fmt.Sprintf("%.2f %s", v, unit)
}

func formatPlain(v float64) string {
	if v == 0 {
		return "0"
	}
	av := v
	if av < 0 {
		av = -av
	}
	if av >= 100 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.3f", v)
}
