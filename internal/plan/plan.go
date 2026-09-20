// Package plan executes mutating operations with durable progress and rollback.
package plan

import "context"

type progressKey struct{}

// ProgressEvent describes the executor's real plan and step transitions. It
// deliberately exposes names and statuses only; previews may contain paths or
// command details that do not belong in an interactive progress surface.
type ProgressEvent struct {
	Steps  []StepState
	Index  int
	Status StepStatus
}

// ProgressReporter receives executor events from the goroutine that runs the
// plan. Consumers must return promptly; long-running UI work belongs outside
// the reporter.
type ProgressReporter func(ProgressEvent)

// WithProgress attaches an optional observer to one plan execution.
func WithProgress(ctx context.Context, reporter ProgressReporter) context.Context {
	return context.WithValue(ctx, progressKey{}, reporter)
}

func reportProgress(ctx context.Context, event ProgressEvent) {
	reporter, _ := ctx.Value(progressKey{}).(ProgressReporter)
	if reporter != nil {
		reporter(event)
	}
}

// ReportProgress publishes a transition for a service operation that has a
// deliberate non-executor state machine, such as ACME certificate issuance.
// It uses the same context-scoped observer as Executor.
func ReportProgress(ctx context.Context, event ProgressEvent) { reportProgress(ctx, event) }

// StartProgress publishes immutable, display-safe step names for a
// non-executor operation.
func StartProgress(ctx context.Context, names ...string) {
	steps := make([]StepState, len(names))
	for index, name := range names {
		steps[index] = StepState{Name: name, Status: StepPending}
	}
	reportProgress(ctx, ProgressEvent{Steps: steps, Index: -1})
}

type Step struct {
	Name       string
	Preview    string
	Do         func(context.Context) error
	Undo       func(context.Context) error
	Idempotent bool
	// Commit makes all completed prior steps a durable recovery boundary once
	// their journal update succeeds. Later failures roll back only subsequent
	// steps and are reported as inconsistent for operator recovery.
	Commit bool
}

type Plan struct {
	Action string
	Target string
	Steps  []Step
}

type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepRunning    StepStatus = "running"
	StepDone       StepStatus = "done"
	StepFailed     StepStatus = "failed"
	StepRolledBack StepStatus = "rolled_back"
)

type StepState struct {
	Name   string     `json:"name"`
	Status StepStatus `json:"status"`
	Error  string     `json:"error,omitempty"`
}

type Snapshot struct {
	Action string      `json:"action"`
	Target string      `json:"target"`
	Steps  []StepState `json:"steps"`
}

func NewSnapshot(operation Plan) Snapshot {
	states := make([]StepState, len(operation.Steps))
	for index, step := range operation.Steps {
		states[index] = StepState{Name: step.Name, Status: StepPending}
	}
	return Snapshot{Action: operation.Action, Target: operation.Target, Steps: states}
}
