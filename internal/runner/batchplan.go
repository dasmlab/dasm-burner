package runner

import (
	"fmt"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

// Serial / pipeline soft limits. We keep wave count under 9 so the Execute
// pipeline stays readable (steps 1…8) and create pressure is paced.
const (
	MaxBatches = 8
	// largeNamespaceThreshold lives in safety.go (allow-large gate).
)

// BatchPlan is the resolved batching strategy for one apply.
type BatchPlan struct {
	Mode       string `json:"mode"`
	Strategy   string `json:"strategy"` // auto | fixed | sequential | rate
	Size       int    `json:"size"`     // namespaces per batch (batch mode)
	Count      int    `json:"count"`    // number of batches / waves
	Namespaces int    `json:"namespaces"`
	Reason     string `json:"reason"`
}

// PlanBatches resolves how namespaces are sliced for deployment.
//
// Breakpoints (batch mode):
//
//	n ≤ 5     → 1 wave (all at once)
//	n ≤ 10    → 2 waves of ≤5  (requires allow-large for real apply when n>10 is separate)
//	n ≤ 30    → up to 5 waves
//	n ≤ 100   → 8 waves
//	n ≤ 500   → 8 waves
//	n > 500   → 8 waves (2500 → ~313 NS / wave)
//
// An explicit deployment.batchSize is honored when it yields ≤ MaxBatches waves;
// otherwise we auto-bump size so Count stays ≤ MaxBatches.
func PlanBatches(cfg *config.Config, namespaces []topology.Namespace) BatchPlan {
	n := len(namespaces)
	mode := config.DeployBatch
	if cfg != nil && cfg.Deployment.Mode != "" {
		mode = cfg.Deployment.Mode
	}
	plan := BatchPlan{Mode: mode, Namespaces: n}

	switch mode {
	case config.DeploySequential:
		plan.Strategy = "sequential"
		plan.Size = 1
		plan.Count = n
		plan.Reason = "sequential: one namespace per batch"
		if n > MaxBatches {
			plan.Reason += fmt.Sprintf(" (warning: %d waves > serial soft limit %d — prefer mode=batch)", n, MaxBatches)
		}
		return plan
	case config.DeployRate:
		plan.Strategy = "rate"
		plan.Size = 1
		plan.Count = n
		plan.Reason = "rate: one namespace per tick"
		return plan
	}

	configured := 0
	if cfg != nil {
		configured = cfg.Deployment.BatchSize
	}
	size, strategy, reason := recommendBatchSize(n, configured)
	plan.Size = size
	plan.Strategy = strategy
	plan.Reason = reason
	plan.Count = batchCount(n, size)
	return plan
}

func recommendBatchSize(n, configured int) (size int, strategy, reason string) {
	if n <= 0 {
		return 0, "auto", "empty"
	}

	autoSize, autoReason := autoBatchSize(n)

	if configured > 0 {
		fixedCount := batchCount(n, configured)
		if fixedCount <= MaxBatches {
			return configured, "fixed", fmt.Sprintf("configured batchSize=%d → %d wave(s)", configured, fixedCount)
		}
		// Fixed size would exceed serial soft limit — bump to auto.
		return autoSize, "auto", fmt.Sprintf(
			"configured batchSize=%d would create %d waves (>%d); using %d → %d wave(s). %s",
			configured, fixedCount, MaxBatches, autoSize, batchCount(n, autoSize), autoReason,
		)
	}
	return autoSize, "auto", autoReason
}

func autoBatchSize(n int) (size int, reason string) {
	switch {
	case n <= 5:
		return n, fmt.Sprintf("≤5 NS: single wave of %d", n)
	case n <= 10:
		// Two waves of ≤5 (e.g. 10 → 5+5, 9 → 5+4).
		size = (n + 1) / 2
		if size > 5 {
			size = 5
		}
		return size, fmt.Sprintf("≤10 NS: 2 waves of ≤5 (size=%d)", size)
	case n <= 30:
		// Prefer ~5 waves, never more than MaxBatches.
		waves := 5
		if n < waves {
			waves = n
		}
		size = ceilDiv(n, waves)
		return size, fmt.Sprintf("≤30 NS: %d waves of ~%d (objects paced per wave)", batchCount(n, size), size)
	case n <= 100:
		size = ceilDiv(n, MaxBatches)
		return size, fmt.Sprintf("≤100 NS: %d waves of ~%d", MaxBatches, size)
	case n <= 500:
		size = ceilDiv(n, MaxBatches)
		return size, fmt.Sprintf("≤500 NS: %d waves of ~%d", MaxBatches, size)
	default:
		// 2500 → ceil(2500/8)=313
		size = ceilDiv(n, MaxBatches)
		return size, fmt.Sprintf("large (%d NS): %d waves of ~%d (target mix pacing)", n, MaxBatches, size)
	}
}

func batchCount(n, size int) int {
	if n <= 0 {
		return 0
	}
	if size <= 0 {
		return 1
	}
	return ceilDiv(n, size)
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return a
	}
	return (a + b - 1) / b
}

// ApplyBatchPlan mutates cfg.Deployment.BatchSize to the resolved plan size for
// batch mode so downstream SplitBatches / kube-burner render stay consistent.
func ApplyBatchPlan(cfg *config.Config, plan BatchPlan) {
	if cfg == nil {
		return
	}
	if plan.Mode == config.DeployBatch && plan.Size > 0 {
		cfg.Deployment.BatchSize = plan.Size
	}
}

// SplitByPlan slices namespaces using PlanBatches.
func SplitByPlan(cfg *config.Config, namespaces []topology.Namespace) ([][]topology.Namespace, BatchPlan) {
	plan := PlanBatches(cfg, namespaces)
	switch plan.Mode {
	case config.DeploySequential, config.DeployRate:
		out := make([][]topology.Namespace, 0, len(namespaces))
		for _, ns := range namespaces {
			out = append(out, []topology.Namespace{ns})
		}
		return out, plan
	default:
		return SplitBatches(namespaces, plan.Size), plan
	}
}
