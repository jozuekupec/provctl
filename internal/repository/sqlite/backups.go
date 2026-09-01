package sqlite

import (
	"context"
	"fmt"
	"time"

	"provctl/internal/domain"
)

func (repository *Repository) ListBackups(ctx context.Context, subscriptionID int64) ([]domain.Backup, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT id, subscription_id, path, COALESCE(size_bytes, 0), status, started_at, COALESCE(finished_at, '') FROM backups WHERE subscription_id = ? ORDER BY started_at DESC`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()
	var backups []domain.Backup
	for rows.Next() {
		backup, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backups: %w", err)
	}
	return backups, nil
}

func (repository *Repository) BackupByID(ctx context.Context, subscriptionID, id int64) (domain.Backup, error) {
	row := repository.DB.QueryRowContext(ctx, `SELECT id, subscription_id, path, COALESCE(size_bytes, 0), status, started_at, COALESCE(finished_at, '') FROM backups WHERE subscription_id = ? AND id = ?`, subscriptionID, id)
	backup, err := scanBackup(row)
	if err != nil {
		return domain.Backup{}, fmt.Errorf("find backup %d: %w", id, err)
	}
	return backup, nil
}

func (repository *Repository) CreateBackup(ctx context.Context, backup domain.Backup) (int64, error) {
	result, err := repository.DB.ExecContext(ctx, `INSERT INTO backups (subscription_id, path, status, started_at) VALUES (?, ?, ?, ?)`, backup.SubscriptionID, backup.Path, backup.Status, backup.StartedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert backup: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read backup ID: %w", err)
	}
	return id, nil
}

func (repository *Repository) FinishBackup(ctx context.Context, id, sizeBytes int64, status string) error {
	if status != "complete" && status != "failed" {
		return fmt.Errorf("invalid final backup status %q", status)
	}
	result, err := repository.DB.ExecContext(ctx, `UPDATE backups SET size_bytes = ?, status = ?, finished_at = ? WHERE id = ? AND status = 'running'`, sizeBytes, status, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("finish backup %d: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count backup update: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("running backup %d not found", id)
	}
	return nil
}

type backupScanner interface{ Scan(...any) error }

func scanBackup(scanner backupScanner) (domain.Backup, error) {
	var backup domain.Backup
	var startedAt, finishedAt string
	if err := scanner.Scan(&backup.ID, &backup.SubscriptionID, &backup.Path, &backup.SizeBytes, &backup.Status, &startedAt, &finishedAt); err != nil {
		return domain.Backup{}, fmt.Errorf("scan backup: %w", err)
	}
	var err error
	if backup.StartedAt, err = time.Parse(time.RFC3339, startedAt); err != nil {
		return domain.Backup{}, fmt.Errorf("parse backup start time: %w", err)
	}
	if finishedAt != "" {
		if backup.FinishedAt, err = time.Parse(time.RFC3339, finishedAt); err != nil {
			return domain.Backup{}, fmt.Errorf("parse backup finish time: %w", err)
		}
	}
	return backup, nil
}
