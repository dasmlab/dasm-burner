package runner

import (
	"testing"

	"github.com/dasmlab/dasm-burner/internal/config"
	"github.com/dasmlab/dasm-burner/internal/topology"
)

func nsList(n int) []topology.Namespace {
	out := make([]topology.Namespace, n)
	for i := range out {
		out[i].Index = i + 1
		out[i].Name = "ns"
	}
	return out
}

// lightDensity is the known-good 2R:2S:1P mix that calibrated ~313 NS waves.
func lightDensity() *config.Config {
	cfg := config.StartingTemplate()
	cfg.Deployment.Mode = config.DeployBatch
	cfg.Deployment.BatchSize = 0
	cfg.Topology.Routes.PerNamespace = 2
	cfg.Topology.Services.PerNamespace = 2
	cfg.Topology.Workloads.ReplicasPerService = 1
	return cfg
}

func dense3Replica() *config.Config {
	cfg := lightDensity()
	cfg.Topology.Workloads.ReplicasPerService = 3
	return cfg
}

func TestAutoBatchBreakpoints(t *testing.T) {
	cases := []struct {
		n         int
		wantSize  int
		wantCount int
	}{
		{1, 1, 1},
		{2, 2, 1},
		{5, 5, 1},
		{6, 3, 2},  // ≤10 → 2 waves of ≤5
		{10, 5, 2}, // 2×5
		{11, 3, 4}, // ≤30 → 5 waves preferred: ceil(11/5)=3 → 4 waves
		{30, 6, 5}, // 5×6
		{100, 13, 8},
		{500, 63, 8},
		{2500, 313, 8},
	}
	for _, tc := range cases {
		cfg := lightDensity()
		cfg.Topology.Namespaces.Count = tc.n
		plan := PlanBatches(cfg, nsList(tc.n))
		if plan.Size != tc.wantSize || plan.Count != tc.wantCount {
			t.Fatalf("n=%d: got size=%d count=%d want size=%d count=%d (%s)",
				tc.n, plan.Size, plan.Count, tc.wantSize, tc.wantCount, plan.Reason)
		}
		if plan.Count > SoftMaxBatches && tc.n <= 2500 && plan.ObjectsPerNS <= 6 {
			t.Fatalf("n=%d light density: count %d exceeds SoftMaxBatches %d", tc.n, plan.Count, SoftMaxBatches)
		}
		batches := SplitBatches(nsList(tc.n), plan.Size)
		if len(batches) != tc.wantCount {
			t.Fatalf("n=%d: split len=%d want %d", tc.n, len(batches), tc.wantCount)
		}
	}
}

func TestDensityCapsThreeReplicaWave(t *testing.T) {
	// 2R:2S:3P → 10 units/NS. Target 1878 → max 187 NS/wave.
	// 2500 → ceil(2500/187)=14 waves (was 8×313 = 3130 objects; now ~1870).
	cfg := dense3Replica()
	cfg.Topology.Namespaces.Count = 2500
	plan := PlanBatches(cfg, nsList(2500))
	if plan.ObjectsPerNS != 10 {
		t.Fatalf("objectsPerNS=%d want 10", plan.ObjectsPerNS)
	}
	if plan.Size > 187 {
		t.Fatalf("size=%d want ≤187 (%s)", plan.Size, plan.Reason)
	}
	if plan.Count < 12 || plan.Count > HardMaxBatches {
		t.Fatalf("count=%d want ~14 (≤%d) (%s)", plan.Count, HardMaxBatches, plan.Reason)
	}
	if plan.ObjectsPerWave > TargetWaveObjects+plan.ObjectsPerNS { // allow one-NS rounding
		t.Fatalf("objectsPerWave=%d over target %d", plan.ObjectsPerWave, TargetWaveObjects)
	}
}

func TestDensityKeepsLightMixAt313(t *testing.T) {
	cfg := lightDensity()
	cfg.Topology.Namespaces.Count = 2500
	plan := PlanBatches(cfg, nsList(2500))
	if plan.Size != 313 || plan.Count != 8 {
		t.Fatalf("got %+v", plan)
	}
	if plan.ObjectsPerWave != 313*6 {
		t.Fatalf("objectsPerWave=%d", plan.ObjectsPerWave)
	}
}

func TestFixedBatchRespectsHardMaxWaves(t *testing.T) {
	cfg := lightDensity()
	cfg.Deployment.BatchSize = 1 // would be 2500 waves
	plan := PlanBatches(cfg, nsList(2500))
	if plan.Strategy != "auto" {
		t.Fatalf("expected auto bump, got %+v", plan)
	}
	if plan.Count != 8 || plan.Size != 313 {
		t.Fatalf("got %+v", plan)
	}
}

func TestFixedBatchHonoredWhenUnderCap(t *testing.T) {
	cfg := lightDensity()
	cfg.Deployment.BatchSize = 2
	plan := PlanBatches(cfg, nsList(6))
	if plan.Strategy != "fixed" || plan.Size != 2 || plan.Count != 3 {
		t.Fatalf("got %+v", plan)
	}
}

func TestSplitByPlanSequential(t *testing.T) {
	cfg := lightDensity()
	cfg.Deployment.Mode = config.DeploySequential
	batches, plan := SplitByPlan(cfg, nsList(4))
	if plan.Count != 4 || len(batches) != 4 {
		t.Fatalf("%+v len=%d", plan, len(batches))
	}
}

func TestObjectsPerNS(t *testing.T) {
	if n := objectsPerNS(lightDensity()); n != 6 {
		t.Fatalf("light=%d", n)
	}
	if n := objectsPerNS(dense3Replica()); n != 10 {
		t.Fatalf("dense=%d", n)
	}
}
