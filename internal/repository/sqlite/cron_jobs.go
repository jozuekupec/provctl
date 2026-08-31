package sqlite

import (
	"context"
	"fmt"
	"time"

	"provctl/internal/domain"
)

func (repository *Repository) ListCronJobs(ctx context.Context, subscriptionID int64) ([]domain.CronJob, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT id, subscription_id, schedule, command, enabled, COALESCE(comment, '') FROM cron_jobs WHERE subscription_id = ? ORDER BY id`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer rows.Close()
	var jobs []domain.CronJob
	for rows.Next() {
		var job domain.CronJob
		if err := rows.Scan(&job.ID, &job.SubscriptionID, &job.Schedule, &job.Command, &job.Enabled, &job.Comment); err != nil {
			return nil, fmt.Errorf("scan cron job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron jobs: %w", err)
	}
	return jobs, nil
}

func (repository *Repository) CreateCronJob(ctx context.Context, job domain.CronJob) (int64, error) {
	result, err := repository.DB.ExecContext(ctx, `INSERT INTO cron_jobs (subscription_id, schedule, command, enabled, comment, created_at) VALUES (?, ?, ?, ?, ?, ?)`, job.SubscriptionID, job.Schedule, job.Command, job.Enabled, nullable(job.Comment), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert cron job: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read cron job ID: %w", err)
	}
	return id, nil
}

func (repository *Repository) DeleteCronJob(ctx context.Context, subscriptionID, id int64) error {
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM cron_jobs WHERE subscription_id = ? AND id = ?`, subscriptionID, id)
	if err != nil {
		return fmt.Errorf("delete cron job %d: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count cron job delete: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("cron job %d not found", id)
	}
	return nil
}
