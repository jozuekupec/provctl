package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"provctl/internal/domain"
)

func (repository *Repository) SubscriptionExists(ctx context.Context, name string) (bool, error) {
	var value int
	err := repository.DB.QueryRowContext(ctx, `SELECT 1 FROM subscriptions WHERE name = ?`, name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query subscription %q: %w", name, err)
	}
	return true, nil
}

func (repository *Repository) CreateSubscription(ctx context.Context, subscription domain.Subscription) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := repository.DB.ExecContext(ctx, `INSERT INTO subscriptions (name, unix_user, unix_uid, home, status, php_version, php_max_children, php_memory_limit, php_upload_max, php_max_exec_time, ssh_access, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?)`, subscription.Name, subscription.UnixUser, subscription.UnixUID, subscription.Home, nullable(subscription.PHPVersion), subscription.PHPMaxChildren, subscription.PHPMemoryLimit, subscription.PHPUploadMax, subscription.PHPMaxExecTime, subscription.SSHAccess, now, now)
	if err != nil {
		return fmt.Errorf("insert subscription %q: %w", subscription.Name, err)
	}
	return nil
}

func (repository *Repository) DeleteSubscription(ctx context.Context, name string) error {
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM subscriptions WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete subscription %q: %w", name, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count subscription delete %q: %w", name, err)
	}
	if rows != 1 {
		return fmt.Errorf("subscription %q not found", name)
	}
	return nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
