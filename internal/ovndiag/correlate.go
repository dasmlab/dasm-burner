package ovndiag

import (
	"fmt"
	"time"
)

// CorrelateBatch looks for multi-category degradation on the same node near a batch marker.
func CorrelateBatch(findings []Finding, batchID int, now time.Time) []Finding {
	if batchID <= 0 {
		return nil
	}
	type agg struct {
		cats map[Category]int
		sevs []Severity
		ids  []string
	}
	byNode := map[string]*agg{}
	for _, f := range findings {
		if f.Severity == SevInfo {
			continue
		}
		if f.Node == "" {
			continue
		}
		a := byNode[f.Node]
		if a == nil {
			a = &agg{cats: map[Category]int{}}
			byNode[f.Node] = a
		}
		a.cats[f.Category]++
		a.sevs = append(a.sevs, f.Severity)
		a.ids = append(a.ids, f.RuleID)
	}
	var out []Finding
	for node, a := range byNode {
		if len(a.cats) < 2 {
			continue
		}
		sev := SevWarning
		for _, s := range a.sevs {
			if s == SevCritical || s == SevError {
				sev = SevCritical
				break
			}
		}
		out = append(out, Finding{
			ID:        fmt.Sprintf("%s-%s-%d-%d", RuleCorrelatedBatch, node, batchID, now.Unix()),
			RuleID:    RuleCorrelatedBatch,
			Severity:  sev,
			Category:  CatCorrelate,
			Node:      node,
			Component: "correlation",
			FirstSeen: now,
			LastSeen:  now,
			Count:     len(a.ids),
			Summary:   fmt.Sprintf("Correlated OVN degradation on %s during batch %d", node, batchID),
			Evidence: []Evidence{
				{Label: "categories", Current: fmt.Sprintf("%d distinct", len(a.cats))},
				{Label: "rules", Current: joinLimited(a.ids, 8)},
				{Label: "batch", Current: fmt.Sprintf("%d", batchID)},
			},
			BatchID: batchID,
			Why: fmt.Sprintf(
				"Multiple diagnostic categories fired on the same node near batch %d — earliest multi-signal evidence of OVN-Kube stress.",
				batchID,
			),
		})
	}
	return out
}

func joinLimited(ss []string, n int) string {
	if len(ss) > n {
		ss = ss[:n]
	}
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
