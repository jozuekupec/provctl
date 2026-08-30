package sqlite

import (
	"context"
	"fmt"

	"provctl/internal/domain"
)

func (repository *Repository) ListDatabases(ctx context.Context, subscriptionID int64) ([]domain.Database, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT id, subscription_id, name, db_user, db_host, charset, collation FROM databases WHERE subscription_id = ? ORDER BY name`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	defer rows.Close()
	var databases []domain.Database
	for rows.Next() {
		var database domain.Database
		if err := rows.Scan(&database.ID, &database.SubscriptionID, &database.Name, &database.User, &database.Host, &database.Charset, &database.Collation); err != nil {
			return nil, fmt.Errorf("scan database: %w", err)
		}
		databases = append(databases, database)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate databases: %w", err)
	}
	return databases, nil
}

func (repository *Repository) CreateDatabase(ctx context.Context, database domain.Database) error {
	_, err := repository.DB.ExecContext(ctx, `INSERT INTO databases (subscription_id, name, db_user, db_host, charset, collation, created_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`, database.SubscriptionID, database.Name, database.User, database.Host, database.Charset, database.Collation)
	if err != nil {
		return fmt.Errorf("insert database %q: %w", database.Name, err)
	}
	return nil
}

func (repository *Repository) DeleteDatabase(ctx context.Context, subscriptionID int64, name string) error {
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM databases WHERE subscription_id = ? AND name = ?`, subscriptionID, name)
	if err != nil {
		return fmt.Errorf("delete database %q: %w", name, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count database delete: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("database %q not found", name)
	}
	return nil
}
