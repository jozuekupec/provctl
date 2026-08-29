package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"provctl/internal/plan"
)

func TestOperationJournal_PersistsRunningOperation(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	journal := OperationJournal{DB: repository.DB}
	snapshot := plan.Snapshot{Action: "subscription.create", Target: "acme", Steps: []plan.StepState{{Name: "create user", Status: plan.StepPending}}}
	id, err := journal.Start(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	running, err := journal.RunningOperations(context.Background())
	if err != nil {
		t.Fatalf("RunningOperations() error = %v", err)
	}
	if len(running) != 1 || running[0].ID != id || running[0].Snapshot.Steps[0].Name != "create user" {
		t.Fatalf("RunningOperations() = %#v, want operation %d", running, id)
	}
	snapshot.Steps[0].Status = plan.StepDone
	if err := journal.Update(context.Background(), id, plan.OperationDone, snapshot, ""); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	running, err = journal.RunningOperations(context.Background())
	if err != nil {
		t.Fatalf("RunningOperations() error = %v", err)
	}
	if len(running) != 0 {
		t.Errorf("RunningOperations() = %#v, want none", running)
	}
}
