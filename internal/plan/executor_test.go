package plan

import (
	"context"
	"errors"
	"testing"

	"provctl/internal/system"
)

type testJournal struct {
	records []record
	nextID  int64
}
type record struct {
	status   OperationStatus
	snapshot Snapshot
	error    string
}

func (journal *testJournal) Start(_ context.Context, snapshot Snapshot) (int64, error) {
	journal.nextID++
	journal.records = append(journal.records, record{OperationRunning, snapshot, ""})
	return journal.nextID, nil
}
func (journal *testJournal) Update(_ context.Context, _ int64, status OperationStatus, snapshot Snapshot, failure string) error {
	journal.records = append(journal.records, record{status, snapshot, failure})
	return nil
}

type testLocker struct{ locked bool }

func (locker *testLocker) Lock(_ context.Context, _ string) (system.Unlock, error) {
	locker.locked = true
	return func() error { locker.locked = false; return nil }, nil
}

func TestExecutor_RollsBackCompletedStepsInReverseOrder(t *testing.T) {
	var calls []string
	journal, locker := &testJournal{}, &testLocker{}
	executor := Executor{Journal: journal, Locker: locker}
	_, err := executor.Run(context.Background(), Plan{Action: "test", Target: "target", Steps: []Step{
		{Name: "first", Do: func(context.Context) error { calls = append(calls, "do first"); return nil }, Undo: func(context.Context) error { calls = append(calls, "undo first"); return nil }},
		{Name: "second", Do: func(context.Context) error { calls = append(calls, "do second"); return errors.New("boom") }},
	}})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if got, want := len(calls), 3; got != want {
		t.Fatalf("calls = %#v, want %d calls", calls, want)
	}
	if calls[2] != "undo first" {
		t.Errorf("rollback calls = %#v, want reverse undo", calls)
	}
	if journal.records[len(journal.records)-1].status != OperationRolledBack {
		t.Errorf("final status = %s, want rolled_back", journal.records[len(journal.records)-1].status)
	}
	if locker.locked {
		t.Error("locker remains held")
	}
}

func TestExecutor_MarksInconsistentWhenUndoFails(t *testing.T) {
	journal, locker := &testJournal{}, &testLocker{}
	_, err := (Executor{Journal: journal, Locker: locker}).Run(context.Background(), Plan{Action: "test", Target: "target", Steps: []Step{
		{Name: "first", Do: func(context.Context) error { return nil }, Undo: func(context.Context) error { return errors.New("undo failed") }},
		{Name: "second", Do: func(context.Context) error { return errors.New("boom") }},
	}})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if journal.records[len(journal.records)-1].status != OperationInconsistent {
		t.Errorf("final status = %s, want inconsistent", journal.records[len(journal.records)-1].status)
	}
}

func TestExecutor_MarksInconsistentWhenIrreversibleStepCompleted(t *testing.T) {
	journal, locker := &testJournal{}, &testLocker{}
	_, err := (Executor{Journal: journal, Locker: locker}).Run(context.Background(), Plan{Action: "test", Target: "target", Steps: []Step{
		{Name: "remove data", Do: func(context.Context) error { return nil }},
		{Name: "second", Do: func(context.Context) error { return errors.New("boom") }},
	}})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if journal.records[len(journal.records)-1].status != OperationInconsistent {
		t.Errorf("final status = %s, want inconsistent", journal.records[len(journal.records)-1].status)
	}
}
