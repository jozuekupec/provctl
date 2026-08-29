// Package plan executes mutating operations with durable progress and rollback.
package plan

import "context"

type Step struct {
	Name       string
	Preview    string
	Do         func(context.Context) error
	Undo       func(context.Context) error
	Idempotent bool
}

type Plan struct {
	Action string
	Target string
	Steps  []Step
}

type StepStatus string

const (
	StepPending    StepStatus = "pending"
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
