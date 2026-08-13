package ovndiag

import (
	"testing"
	"time"
)

func TestWorstState(t *testing.T) {
	if got := worstState(StateHealthy, StateWarning, StateCritical); got != StateCritical {
		t.Fatalf("got %s", got)
	}
}

func TestWhyHealthy(t *testing.T) {
	s := &Snapshot{OverallState: StateHealthy, HealthyCount: 3}
	if why(s) == "" {
		t.Fatal("expected why text")
	}
}

func TestCorrelateBatch(t *testing.T) {
	now := time.Now()
	fs := []Finding{
		{Node: "w1", Category: CatOVNKube, Severity: SevWarning, RuleID: RuleOVNKubeRestart},
		{Node: "w1", Category: CatLog, Severity: SevWarning, RuleID: RuleLogAnomaly},
		{Node: "w2", Category: CatNode, Severity: SevWarning, RuleID: RuleMemoryPressure},
	}
	out := CorrelateBatch(fs, 7, now)
	if len(out) != 1 {
		t.Fatalf("want 1 correlated finding, got %d", len(out))
	}
	if out[0].RuleID != RuleCorrelatedBatch || out[0].Node != "w1" {
		t.Fatalf("unexpected %+v", out[0])
	}
}

func TestClassifyLog(t *testing.T) {
	if got := classifyLog("error: connection refused to northd"); got != "CONNECTION" && got != "ERROR" {
		t.Fatalf("got %q", got)
	}
	if got := classifyLog("sbdb transaction failed"); got != "DATABASE" {
		t.Fatalf("got %q want DATABASE", got)
	}
	if got := classifyLog("hello world"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
