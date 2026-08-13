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
		cfg := config.StartingTemplate()
		cfg.Deployment.Mode = config.DeployBatch
		cfg.Deployment.BatchSize = 0
		cfg.Topology.Namespaces.Count = tc.n
		plan := PlanBatches(cfg, nsList(tc.n))
		if plan.Size != tc.wantSize || plan.Count != tc.wantCount {
			t.Fatalf("n=%d: got size=%d count=%d want size=%d count=%d (%s)",
				tc.n, plan.Size, plan.Count, tc.wantSize, tc.wantCount, plan.Reason)
		}
		if plan.Count > MaxBatches {
			t.Fatalf("n=%d: count %d exceeds MaxBatches %d", tc.n, plan.Count, MaxBatches)
		}
		batches := SplitBatches(nsList(tc.n), plan.Size)
		if len(batches) != tc.wantCount {
			t.Fatalf("n=%d: split len=%d want %d", tc.n, len(batches), tc.wantCount)
		}
	}
}

func TestFixedBatchRespectsMaxWaves(t *testing.T) {
	cfg := config.StartingTemplate()
	cfg.Deployment.Mode = config.DeployBatch
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
	cfg := config.StartingTemplate()
	cfg.Deployment.Mode = config.DeployBatch
	cfg.Deployment.BatchSize = 2
	plan := PlanBatches(cfg, nsList(6))
	if plan.Strategy != "fixed" || plan.Size != 2 || plan.Count != 3 {
		t.Fatalf("got %+v", plan)
	}
}

func TestSplitByPlanSequential(t *testing.T) {
	cfg := config.StartingTemplate()
	cfg.Deployment.Mode = config.DeploySequential
	batches, plan := SplitByPlan(cfg, nsList(4))
	if plan.Count != 4 || len(batches) != 4 {
		t.Fatalf("%+v len=%d", plan, len(batches))
	}
}
