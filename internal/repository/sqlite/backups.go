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
