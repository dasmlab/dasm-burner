package ovndiag

import "testing"

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
