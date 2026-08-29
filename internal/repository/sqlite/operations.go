package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"provctl/internal/plan"
)

// OperationJournal persists executor progress in the operations table.
type OperationJournal struct{ DB *sql.DB }

func (journal OperationJournal) Start(ctx context.Context, snapshot plan.Snapshot) (int64, error) {
	contents, err := json.Marshal(snapshot)
	if err != nil {
		return 0, fmt.Errorf("encode operation plan: %w", err)
	}
	result, err := journal.DB.ExecContext(ctx, `INSERT INTO operations (action, target, actor, status, plan_json, started_at) VALUES (?, ?, ?, ?, ?, ?)`, snapshot.Action, snapshot.Target, "root", plan.OperationRunning, string(contents), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert operation: %w", err)
	}
	return result.LastInsertId()
}

func (journal OperationJournal) Update(ctx context.Context, id int64, status plan.OperationStatus, snapshot plan.Snapshot, operationError string) error {
	contents, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode operation plan: %w", err)
	}
	var finishedAt any
	if status != plan.OperationRunning {
		finishedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := journal.DB.ExecContext(ctx, `UPDATE operations SET status = ?, plan_json = ?, error = ?, finished_at = ? WHERE id = ?`, status, string(contents), nullableString(operationError), finishedAt, id)
	if err != nil {
		return fmt.Errorf("update operation %d: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count operation update %d: %w", id, err)
	}
	if rows != 1 {
		return fmt.Errorf("operation %d not found", id)
	}
	return nil
}

type Operation struct {
	ID       int64
	Action   string
	Target   string
	Status   plan.OperationStatus
	Snapshot plan.Snapshot
	Error    string
}

// RunningOperations returns interrupted operations for startup warnings.
func (journal OperationJournal) RunningOperations(ctx context.Context) ([]Operation, error) {
	rows, err := journal.DB.QueryContext(ctx, `SELECT id, action, target, status, plan_json, COALESCE(error, '') FROM operations WHERE status = ? ORDER BY id`, plan.OperationRunning)
	if err != nil {
		return nil, fmt.Errorf("list running operations: %w", err)
	}
	defer rows.Close()
	var operations []Operation
	for rows.Next() {
		var operation Operation
		var contents string
		if err := rows.Scan(&operation.ID, &operation.Action, &operation.Target, &operation.Status, &contents, &operation.Error); err != nil {
			return nil, fmt.Errorf("scan running operation: %w", err)
		}
		if err := json.Unmarshal([]byte(contents), &operation.Snapshot); err != nil {
			return nil, fmt.Errorf("decode operation %d: %w", operation.ID, err)
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate running operations: %w", err)
	}
	return operations, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ plan.Journal = OperationJournal{}
