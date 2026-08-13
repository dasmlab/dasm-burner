package runner

import (
	"fmt"
	"time"

	"github.com/dasmlab/dasm-burner/internal/kube"
)

type Phase string

const (
	PhasePrecheck         Phase = "PRECHECK"
	PhaseBaseline         Phase = "BASELINE"
	PhaseBatchStart       Phase = "BATCH_START"
	PhaseObjectCreation   Phase = "OBJECT_CREATION"
	PhaseWaitForReady     Phase = "WAIT_FOR_READY"
	PhaseBatchMeasurement Phase = "BATCH_MEASUREMENT"
	PhaseHealthCheck      Phase = "HEALTH_CHECK"
	PhaseFinalSettle      Phase = "FINAL_SETTLE"
	PhaseFinalMeasurement Phase = "FINAL_MEASUREMENT"
	PhaseReport           Phase = "REPORT"
	PhaseDone             Phase = "DONE"
	PhaseAborted          Phase = "ABORTED"
)

type PhaseEvent struct {
	Phase   Phase     `json:"phase"`
	At      time.Time `json:"at"`
	Message string    `json:"message,omitempty"`
	Batch   int       `json:"batch,omitempty"`
}

type CreateCounts struct {
	Created  int `json:"created"`
	Existing int `json:"existing"`
	Failed   int `json:"failed"`
}

type BatchReport struct {
	ID          int              `json:"id"`
	Namespaces  int              `json:"namespaces"`
	Services    int              `json:"services"`
	Routes      int              `json:"routes"`
	Deployments int              `json:"deployments"`
	Pods        int              `json:"pods"`
	NS          CreateCounts     `json:"namespaceCreates"`
	Svc         CreateCounts     `json:"serviceCreates"`
	Rt          CreateCounts     `json:"routeCreates"`
	Dep         CreateCounts     `json:"deploymentCreates"`
	CreateDur   time.Duration    `json:"createDuration"`
	Ready       kube.ReadyStats  `json:"ready"`
	Convergence kube.Convergence `json:"convergence"`
	Health      kube.Health      `json:"health,omitempty"`
	HealthLevel string           `json:"healthLevel,omitempty"`
	Errors      []string         `json:"errors,omitempty"`
}

type Report struct {
	RunID       string           `json:"runId"`
	Seed        int64            `json:"seed"`
	Mode        string           `json:"mode"`
	DryRun      bool             `json:"dryRun"`
	Cluster     string           `json:"cluster,omitempty"`
	Started     time.Time        `json:"started"`
	Finished    time.Time        `json:"finished"`
	Duration    time.Duration    `json:"duration"`
	Phases      []PhaseEvent     `json:"phases"`
	Batches     []BatchReport    `json:"batches"`
	Convergence kube.Convergence `json:"convergence"`
	Health      kube.Health      `json:"health,omitempty"`
	Aborted     bool             `json:"aborted,omitempty"`
	AbortReason string           `json:"abortReason,omitempty"`
	Errors      []string         `json:"errors,omitempty"`
}

func (r *Report) event(phase Phase, batch int, msg string) {
	r.Phases = append(r.Phases, PhaseEvent{Phase: phase, At: time.Now(), Message: msg, Batch: batch})
}

func (c CreateCounts) String() string {
	return fmt.Sprintf("created=%d existing=%d failed=%d", c.Created, c.Existing, c.Failed)
}
