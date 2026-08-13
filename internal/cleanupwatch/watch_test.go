package cleanupwatch

import (
	"testing"
	"time"
)

func TestDetectNodeTransitionNotReadyThenRecover(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	prev := map[string]time.Time{}
	still, incs := DetectNodeTransition(prev, now, []string{"worker-a"})
	if len(incs) != 1 || incs[0].Kind != "node_not_ready" {
		t.Fatalf("want node_not_ready, got %+v", incs)
	}
	if _, ok := still["worker-a"]; !ok {
		t.Fatal("expected worker-a tracked")
	}
	later := now.Add(45 * time.Second)
	still2, incs2 := DetectNodeTransition(still, later, nil)
	if len(still2) != 0 {
		t.Fatalf("expected cleared, got %+v", still2)
	}
	if len(incs2) != 1 || incs2[0].Kind != "node_recovered" {
		t.Fatalf("want node_recovered, got %+v", incs2)
	}
	if !contains(incs2[0].Message, "45s") {
		t.Fatalf("expected duration in message: %s", incs2[0].Message)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	t.Parallel()
	s := summarize(Observation{})
	if !contains(s, "No node/monitoring incidents") {
		t.Fatalf("got %q", s)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
