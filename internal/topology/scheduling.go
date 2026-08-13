package topology

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/dasmlab/dasm-burner/internal/config"
)

// ApplyScheduling strips tolerations that would allow avoided taints and sets
// a required nodeAffinity so pods also skip nodes labeled like those roles.
// aaustin: infra nodes are tainted node-role.kubernetes.io=infra — do not tolerate it.
func ApplyScheduling(spec *corev1.PodSpec, avoid []config.AvoidTaint) {
	if spec == nil {
		return
	}
	spec.Tolerations = FilterTolerations(spec.Tolerations, avoid)
	if aff := AvoidTaintAffinity(avoid); aff != nil {
		if spec.Affinity == nil {
			spec.Affinity = &corev1.Affinity{}
		}
		spec.Affinity.NodeAffinity = mergeNodeAffinity(spec.Affinity.NodeAffinity, aff)
	}
}

// FilterTolerations drops any toleration that would match an avoided taint.
func FilterTolerations(in []corev1.Toleration, avoid []config.AvoidTaint) []corev1.Toleration {
	if len(avoid) == 0 || len(in) == 0 {
		return in
	}
	var out []corev1.Toleration
	for _, tol := range in {
		if tolerationAllowed(tol, avoid) {
			out = append(out, tol)
		}
	}
	return out
}

func tolerationAllowed(tol corev1.Toleration, avoid []config.AvoidTaint) bool {
	for _, a := range avoid {
		if a.MatchesToleration(tol.Key, tol.Value, string(tol.Effect), string(tol.Operator)) {
			return false
		}
	}
	return true
}

// AvoidTaintAffinity builds requiredDuringScheduling expressions that keep
// pods off nodes carrying the avoided role labels (OCP infra convention).
func AvoidTaintAffinity(avoid []config.AvoidTaint) *corev1.NodeAffinity {
	exprs := avoidLabelExpressions(avoid)
	if len(exprs) == 0 {
		return nil
	}
	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: exprs,
			}},
		},
	}
}

func avoidLabelExpressions(avoid []config.AvoidTaint) []corev1.NodeSelectorRequirement {
	seen := map[string]bool{}
	var exprs []corev1.NodeSelectorRequirement
	addDoesNotExist := func(key string) {
		if key == "" || seen["!"+key] {
			return
		}
		seen["!"+key] = true
		exprs = append(exprs, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpDoesNotExist,
		})
	}
	addNotIn := func(key, value string) {
		id := key + "!=" + value
		if key == "" || value == "" || seen[id] {
			return
		}
		seen[id] = true
		exprs = append(exprs, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpNotIn,
			Values:   []string{value},
		})
	}

	for _, a := range avoid {
		switch {
		case a.Key == "node-role.kubernetes.io" && a.Value == "infra":
			// kubectl form used on this cluster + classic OCP infra label.
			addNotIn(a.Key, a.Value)
			addDoesNotExist("node-role.kubernetes.io/infra")
		case strings.Contains(a.Key, "node-role.kubernetes.io/") && a.Value == "":
			addDoesNotExist(a.Key)
		case a.Value != "":
			addNotIn(a.Key, a.Value)
		default:
			addDoesNotExist(a.Key)
		}
	}
	return exprs
}

func mergeNodeAffinity(existing, extra *corev1.NodeAffinity) *corev1.NodeAffinity {
	if existing == nil {
		return extra
	}
	if extra == nil {
		return existing
	}
	if existing.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		existing.RequiredDuringSchedulingIgnoredDuringExecution = extra.RequiredDuringSchedulingIgnoredDuringExecution
		return existing
	}
	if extra.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return existing
	}
	terms := existing.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) == 0 {
		existing.RequiredDuringSchedulingIgnoredDuringExecution = extra.RequiredDuringSchedulingIgnoredDuringExecution
		return existing
	}
	var extraExprs []corev1.NodeSelectorRequirement
	for _, t := range extra.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		extraExprs = append(extraExprs, t.MatchExpressions...)
	}
	for i := range terms {
		terms[i].MatchExpressions = append(terms[i].MatchExpressions, extraExprs...)
	}
	existing.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = terms
	return existing
}
