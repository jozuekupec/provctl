package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"provctl/internal/domain"
)

func (repository *Repository) DomainExists(ctx context.Context, domain string) (bool, error) {
	var value int
	err := repository.DB.QueryRowContext(ctx, `SELECT 1 FROM domains WHERE name = ?`, domain).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query domain %q: %w", domain, err)
	}
	return true, nil
}

// ListWebsites returns websites belonging to one subscription, ordered by domain.
func (repository *Repository) ListWebsites(ctx context.Context, subscriptionID int64) ([]domain.Website, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT w.id, w.subscription_id, w.type, d.name, COALESCE(w.document_root, ''), COALESCE(w.php_version, ''), w.enabled, w.ssl_enabled, w.force_https, w.hsts FROM websites w JOIN domains d ON d.website_id = w.id AND d.is_primary = 1 WHERE w.subscription_id = ? ORDER BY d.name`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list websites: %w", err)
	}
	defer rows.Close()
	var websites []domain.Website
	for rows.Next() {
		var website domain.Website
		if err := rows.Scan(&website.ID, &website.SubscriptionID, &website.Type, &website.PrimaryDomain, &website.DocumentRoot, &website.PHPVersion, &website.Enabled, &website.SSLEnabled, &website.ForceHTTPS, &website.HSTS); err != nil {
			return nil, fmt.Errorf("scan website: %w", err)
		}
		websites = append(websites, website)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate websites: %w", err)
	}
	return websites, nil
}

func (repository *Repository) CreateWebsite(ctx context.Context, website domain.Website) (int64, error) {
	transaction, err := repository.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin website insert: %w", err)
	}
	defer transaction.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := transaction.ExecContext(ctx, `INSERT INTO websites (subscription_id, type, document_root, php_version, enabled, ssl_enabled, force_https, hsts, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, website.SubscriptionID, website.Type, nullable(website.DocumentRoot), nullable(website.PHPVersion), website.Enabled, website.SSLEnabled, website.ForceHTTPS, website.HSTS, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert website: %w", err)
	}
	websiteID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read website ID: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO domains (website_id, name, is_primary, created_at) VALUES (?, ?, 1, ?)`, websiteID, website.PrimaryDomain, now); err != nil {
		return 0, fmt.Errorf("insert primary domain: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("commit website insert: %w", err)
	}
	return websiteID, nil
}

func (repository *Repository) DeleteWebsite(ctx context.Context, websiteID int64) error {
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM websites WHERE id = ?`, websiteID)
	if err != nil {
		return fmt.Errorf("delete website %d: %w", websiteID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count website delete %d: %w", websiteID, err)
	}
	if rows != 1 {
		return fmt.Errorf("website %d not found", websiteID)
	}
	return nil
}
