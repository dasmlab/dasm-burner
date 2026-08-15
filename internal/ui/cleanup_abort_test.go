package ui

import (
	"testing"
	"time"
)

func TestAbortForCleanupStopsRunning(t *testing.T) {
	canceled := false
	r := &execRun{
		Status: "running",
		Steps:  []runStep{{ID: "b6", Status: stepRunning}},
		cancel: func() { canceled = true },
	}
	r.abortForCleanup()
	if !canceled {
		t.Fatal("expected cancel")
	}
	if r.Status != "aborted" {
		t.Fatalf("status %s", r.Status)
	}
	if r.Error != "stopped so cleanup can run" {
		t.Fatalf("error %q", r.Error)
	}
	if r.Finished == nil {
		t.Fatal("expected finished")
	}
	if r.Steps[0].Status != stepFailed {
		t.Fatalf("step %s", r.Steps[0].Status)
	}
}

func TestAbortForCleanupIdempotentWhenIdle(t *testing.T) {
	r := &execRun{Status: "idle"}
	r.abortForCleanup()
	if r.Status != "idle" {
		t.Fatalf("status %s", r.Status)
	}
}

func TestAbortForCleanupNilSafe(t *testing.T) {
	var r *execRun
	r.abortForCleanup()
}

func TestAbortForCleanupDoesNotDeadlock(t *testing.T) {
	r := &execRun{Status: "running"}
	done := make(chan struct{})
	go func() {
		r.abortForCleanup()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("abortForCleanup deadlocked")
	}
}
