package sqlite

import (
	"context"
	"fmt"
	"time"

	"provctl/internal/domain"
)

func (repository *Repository) ListSSHKeys(ctx context.Context, subscriptionID int64) ([]domain.SSHKey, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT id, subscription_id, COALESCE(comment, ''), fingerprint, public_key FROM ssh_keys WHERE subscription_id = ? ORDER BY id`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list SSH keys: %w", err)
	}
	defer rows.Close()
	var keys []domain.SSHKey
	for rows.Next() {
		var key domain.SSHKey
		if err := rows.Scan(&key.ID, &key.SubscriptionID, &key.Comment, &key.Fingerprint, &key.PublicKey); err != nil {
			return nil, fmt.Errorf("scan SSH key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SSH keys: %w", err)
	}
	return keys, nil
}

func (repository *Repository) CreateSSHKey(ctx context.Context, key domain.SSHKey) (int64, error) {
	result, err := repository.DB.ExecContext(ctx, `INSERT INTO ssh_keys (subscription_id, comment, fingerprint, public_key, created_at) VALUES (?, ?, ?, ?, ?)`, key.SubscriptionID, nullable(key.Comment), key.Fingerprint, key.PublicKey, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert SSH key: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read SSH key ID: %w", err)
	}
	return id, nil
}

func (repository *Repository) DeleteSSHKey(ctx context.Context, subscriptionID int64, fingerprint string) error {
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM ssh_keys WHERE subscription_id = ? AND fingerprint = ?`, subscriptionID, fingerprint)
	if err != nil {
		return fmt.Errorf("delete SSH key %q: %w", fingerprint, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count SSH key delete: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("SSH key %q not found", fingerprint)
	}
	return nil
}

func (repository *Repository) UpdateSSHAccess(ctx context.Context, subscriptionID int64, access string) error {
	result, err := repository.DB.ExecContext(ctx, `UPDATE subscriptions SET ssh_access = ?, updated_at = ? WHERE id = ?`, access, time.Now().UTC().Format(time.RFC3339), subscriptionID)
	if err != nil {
		return fmt.Errorf("update SSH access: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count SSH access update: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("subscription %d not found", subscriptionID)
	}
	return nil
}
