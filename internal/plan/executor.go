package plan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"provctl/internal/audit"
	"provctl/internal/system"
)

type OperationStatus string

const (
	OperationRunning      OperationStatus = "running"
	OperationDone         OperationStatus = "done"
	OperationRolledBack   OperationStatus = "rolled_back"
	OperationInconsistent OperationStatus = "inconsistent"
)

// Journal persists progress before and after each system change.
type Journal interface {
	Start(context.Context, Snapshot) (int64, error)
	Update(context.Context, int64, OperationStatus, Snapshot, string) error
}

type Executor struct {
	Journal Journal
	Locker  system.Locker
	Audit   audit.Writer
}

type ExecutionError struct {
	OperationID int64
	Err         error
}

func (err ExecutionError) Error() string {
	return fmt.Sprintf("operation %d: %v", err.OperationID, err.Err)
}
func (err ExecutionError) Unwrap() error { return err.Err }

func (executor Executor) Run(ctx context.Context, operation Plan) (operationID int64, returnErr error) {
	started := time.Now()
	defer func() {
		if executor.Audit == nil {
			return
		}
		status := "ok"
		if returnErr != nil {
			status = "failed"
		}
		// The audit entry intentionally has no step previews, command arguments,
		// SQL, or error string, because those may contain secrets.
		_ = executor.Audit.Write(context.Background(), audit.Entry{Timestamp: started, Action: operation.Action, Target: operation.Target, Status: status, DurationMS: time.Since(started).Milliseconds(), OperationID: operationID})
	}()
	if executor.Journal == nil || executor.Locker == nil {
		return 0, errors.New("plan executor requires journal and locker")
	}
	if operation.Action == "" || operation.Target == "" {
		return 0, errors.New("plan action and target are required")
	}
	unlock, err := executor.Locker.Lock(ctx, operation.Action+" "+operation.Target)
	if err != nil {
		return 0, err
	}
	defer unlock()
	snapshot := NewSnapshot(operation)
	id, err := executor.Journal.Start(ctx, snapshot)
	if err != nil {
		return 0, fmt.Errorf("start operation journal: %w", err)
	}
	completed := make([]int, 0, len(operation.Steps))
	for index, step := range operation.Steps {
		if step.Do == nil {
			return id, executor.fail(ctx, id, operation, snapshot, completed, index, errors.New("step has no action"))
		}
		if err := step.Do(ctx); err != nil {
			return id, executor.fail(ctx, id, operation, snapshot, completed, index, err)
		}
		snapshot.Steps[index].Status = StepDone
		completed = append(completed, index)
		if err := executor.Journal.Update(ctx, id, OperationRunning, snapshot, ""); err != nil {
			return id, executor.rollback(ctx, id, operation, snapshot, completed, fmt.Errorf("record completed step: %w", err))
		}
	}
	if err := executor.Journal.Update(ctx, id, OperationDone, snapshot, ""); err != nil {
		return id, ExecutionError{OperationID: id, Err: fmt.Errorf("record completed operation: %w", err)}
	}
	return id, nil
}

func (executor Executor) fail(ctx context.Context, id int64, operation Plan, snapshot Snapshot, completed []int, failed int, cause error) error {
	snapshot.Steps[failed].Status, snapshot.Steps[failed].Error = StepFailed, cause.Error()
	_ = executor.Journal.Update(ctx, id, OperationRunning, snapshot, cause.Error())
	return executor.rollback(ctx, id, operation, snapshot, completed, cause)
}

func (executor Executor) rollback(ctx context.Context, id int64, operation Plan, snapshot Snapshot, completed []int, cause error) error {
	var undoErrors []error
	var irreversible []string
	for offset := len(completed) - 1; offset >= 0; offset-- {
		index := completed[offset]
		undo := operation.Steps[index].Undo
		if undo == nil {
			irreversible = append(irreversible, operation.Steps[index].Name)
			continue
		}
		if err := undo(ctx); err != nil {
			undoErrors = append(undoErrors, fmt.Errorf("undo %s: %w", operation.Steps[index].Name, err))
			continue
		}
		snapshot.Steps[index].Status = StepRolledBack
		_ = executor.Journal.Update(ctx, id, OperationRunning, snapshot, cause.Error())
	}
	status := OperationRolledBack
	if len(undoErrors) > 0 || len(irreversible) > 0 {
		status = OperationInconsistent
	}
	if len(irreversible) > 0 {
		undoErrors = append(undoErrors, fmt.Errorf("irreversible completed steps remain: %v", irreversible))
	}
	finalError := errors.Join(append([]error{cause}, undoErrors...)...)
	if err := executor.Journal.Update(ctx, id, status, snapshot, finalError.Error()); err != nil {
		finalError = errors.Join(finalError, fmt.Errorf("record failed operation: %w", err))
	}
	return ExecutionError{OperationID: id, Err: finalError}
}
