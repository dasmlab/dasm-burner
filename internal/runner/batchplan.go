package runner

import (
	"fmt"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

// Serial / pipeline limits. SoftMax is the preferred wave count for light density
// (Execute stage rail stays short). HardMax is the ceiling when replica/route
// density forces smaller waves so we do not dump too many objects into etcd/API
// in one settle window.
const (
	SoftMaxBatches = 8
	HardMaxBatches = 24
	// Deprecated alias — prefer SoftMaxBatches.
	MaxBatches = SoftMaxBatches

	// Known-good create pressure from the 2R:2S:1P × ~313 NS wave that did not
	// knock over apiserver/etcd on the lab clusters (~4 minute wave).
	// units = routes + services + pods per NS → 2+2+2 = 6; 313×6 = 1878.
	TargetWaveObjects = 1878
	ReferenceRoutes   = 2
	ReferenceServices = 2
	ReferenceReplicas = 1
)

// BatchPlan is the resolved batching strategy for one apply.
type BatchPlan struct {
	Mode            string `json:"mode"`
	Strategy        string `json:"strategy"` // auto | fixed | sequential | rate
	Size            int    `json:"size"`    // namespaces per batch (batch mode)
	Count           int    `json:"count"`   // number of batches / waves
	Namespaces      int    `json:"namespaces"`
	ObjectsPerNS    int    `json:"objectsPerNs,omitempty"`
	ObjectsPerWave  int    `json:"objectsPerWave,omitempty"`
	TargetWaveObjs  int    `json:"targetWaveObjects,omitempty"`
	Reason          string `json:"reason"`
}

// objectsPerNS estimates create-pressure units for one namespace.
// Matches the lab rule of thumb: routes + services + pods (svc×replicas).
// Deployments are 1:1 with services and are folded into that service create path
// for pacing; pods dominate the etcd/API spike when replicas rise.
func objectsPerNS(cfg *config.Config) int {
	if cfg == nil {
		return ReferenceRoutes + ReferenceServices + ReferenceServices*ReferenceReplicas
	}
	if cfg.IsObjectPressure() {
		c := cfg.Counts()
		if c.Namespaces <= 0 {
			return 1
		}
		// intended includes the NS itself; per-NS pressure is the rest + 1 NS create.
		per := c.Intended / c.Namespaces
		if per < 1 {
			return 1
		}
		return per
	}
	r := cfg.Topology.Routes.PerNamespace
	s := cfg.Topology.Services.PerNamespace
	rep := cfg.Topology.Workloads.ReplicasPerService
	if r <= 0 {
		r = ReferenceRoutes
	}
	if s <= 0 {
		s = ReferenceServices
	}
	if rep <= 0 {
		rep = ReferenceReplicas
	}
	return r + s + s*rep
}

// PlanBatches resolves how namespaces are sliced for deployment.
//
// Breakpoints (batch mode, light density ≈ 2R:2S:1P):
//
//	n ≤ 5     → 1 wave
//	n ≤ 10    → 2 waves of ≤5
//	n ≤ 30    → up to 5 waves
//	n ≤ 100   → SoftMax waves
//	n ≤ 500   → SoftMax waves
//	n > 500   → SoftMax waves (2500 → ~313 NS / wave at 2:2:1)
//
// Density: wave size is also capped so routes+services+pods per wave stay near
// TargetWaveObjects. 2R:2S:3P on 2500 NS → ~187 NS/wave (~14 waves) instead of
// pushing ~3130 objects into the same settle window that used to hold ~1878.
//
// An explicit deployment.batchSize is honored when it yields ≤ HardMaxBatches
// waves; otherwise we auto-bump size. Over-budget fixed sizes get a warning.
func PlanBatches(cfg *config.Config, namespaces []topology.Namespace) BatchPlan {
	n := len(namespaces)
	mode := config.DeployBatch
	if cfg != nil && cfg.Deployment.Mode != "" {
		mode = cfg.Deployment.Mode
	}
	perNS := objectsPerNS(cfg)
	plan := BatchPlan{
		Mode:           mode,
		Namespaces:     n,
		ObjectsPerNS:   perNS,
		TargetWaveObjs: TargetWaveObjects,
	}

	switch mode {
	case config.DeploySequential:
		plan.Strategy = "sequential"
		plan.Size = 1
		plan.Count = n
		plan.ObjectsPerWave = perNS
		plan.Reason = "sequential: one namespace per batch"
		if n > SoftMaxBatches {
			plan.Reason += fmt.Sprintf(" (warning: %d waves > soft limit %d — prefer mode=batch)", n, SoftMaxBatches)
		}
		return plan
	case config.DeployRate:
		plan.Strategy = "rate"
		plan.Size = 1
		plan.Count = n
		plan.ObjectsPerWave = perNS
		plan.Reason = "rate: one namespace per tick"
		return plan
	}

	configured := 0
	if cfg != nil {
		configured = cfg.Deployment.BatchSize
	}
	size, strategy, reason := recommendBatchSize(n, configured, perNS)
	plan.Size = size
	plan.Strategy = strategy
	plan.Reason = reason
	plan.Count = batchCount(n, size)
	plan.ObjectsPerWave = size * perNS
	return plan
}

func recommendBatchSize(n, configured, perNS int) (size int, strategy, reason string) {
	if n <= 0 {
		return 0, "auto", "empty"
	}
	if perNS < 1 {
		perNS = 1
	}

	autoSize, autoReason := autoBatchSize(n)
	autoSize, densReason := applyDensityCap(n, autoSize, perNS)
	if densReason != "" {
		autoReason = autoReason + "; " + densReason
	}

	if configured > 0 {
		fixedCount := batchCount(n, configured)
		if fixedCount <= HardMaxBatches {
			waveObjs := configured * perNS
			msg := fmt.Sprintf("configured batchSize=%d → %d wave(s) · ~%d objects/wave", configured, fixedCount, waveObjs)
			if waveObjs > TargetWaveObjects {
				msg += fmt.Sprintf(" (warning: above target %d — may stress etcd/API; auto would use %d NS/wave)",
					TargetWaveObjects, autoSize)
			}
			return configured, "fixed", msg
		}
		return autoSize, "auto", fmt.Sprintf(
			"configured batchSize=%d would create %d waves (>%d); using %d → %d wave(s). %s",
			configured, fixedCount, HardMaxBatches, autoSize, batchCount(n, autoSize), autoReason,
		)
	}
	return autoSize, "auto", autoReason
}

func autoBatchSize(n int) (size int, reason string) {
	switch {
	case n <= 5:
		return n, fmt.Sprintf("≤5 NS: single wave of %d", n)
	case n <= 10:
		size = (n + 1) / 2
		if size > 5 {
			size = 5
		}
		return size, fmt.Sprintf("≤10 NS: 2 waves of ≤5 (size=%d)", size)
	case n <= 30:
		waves := 5
		if n < waves {
			waves = n
		}
		size = ceilDiv(n, waves)
		return size, fmt.Sprintf("≤30 NS: %d waves of ~%d (objects paced per wave)", batchCount(n, size), size)
	case n <= 100:
		size = ceilDiv(n, SoftMaxBatches)
		return size, fmt.Sprintf("≤100 NS: %d waves of ~%d", SoftMaxBatches, size)
	case n <= 500:
		size = ceilDiv(n, SoftMaxBatches)
		return size, fmt.Sprintf("≤500 NS: %d waves of ~%d", SoftMaxBatches, size)
	default:
		// 2500 → ceil(2500/8)=313 at light density before density cap.
		size = ceilDiv(n, SoftMaxBatches)
		return size, fmt.Sprintf("large (%d NS): prefer %d waves of ~%d", n, SoftMaxBatches, size)
	}
}

// applyDensityCap shrinks NS/wave so create-pressure units stay near TargetWaveObjects.
func applyDensityCap(n, size, perNS int) (int, string) {
	if size <= 0 || perNS <= 0 {
		return size, ""
	}
	maxNS := TargetWaveObjects / perNS
	if maxNS < 1 {
		maxNS = 1
	}
	if size <= maxNS {
		waveObjs := size * perNS
		if perNS <= ReferenceRoutes+ReferenceServices+ReferenceServices*ReferenceReplicas {
			return size, fmt.Sprintf("~%d objects/wave (units/NS=%d, target≤%d)", waveObjs, perNS, TargetWaveObjects)
		}
		return size, fmt.Sprintf("~%d objects/wave (units/NS=%d ≤ budget at this size)", waveObjs, perNS)
	}

	capped := maxNS
	count := batchCount(n, capped)
	if count > HardMaxBatches {
		capped = ceilDiv(n, HardMaxBatches)
		count = batchCount(n, capped)
	}
	waveObjs := capped * perNS
	return capped, fmt.Sprintf(
		"density units/NS=%d → cap %d NS/wave (~%d objects ≤ target %d) → %d waves (soft=%d hard=%d)",
		perNS, capped, waveObjs, TargetWaveObjects, count, SoftMaxBatches, HardMaxBatches,
	)
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
