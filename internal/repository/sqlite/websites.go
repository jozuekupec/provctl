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
	rows, err := repository.DB.QueryContext(ctx, `SELECT w.id, w.subscription_id, w.type, d.name, COALESCE(w.document_root, ''), COALESCE(w.target, ''), COALESCE(w.redirect_code, 0), COALESCE(w.php_version, ''), w.enabled, w.ssl_enabled, w.force_https, w.hsts FROM websites w JOIN domains d ON d.website_id = w.id AND d.is_primary = 1 WHERE w.subscription_id = ? ORDER BY d.name`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list websites: %w", err)
	}
	defer rows.Close()
	var websites []domain.Website
	for rows.Next() {
		var website domain.Website
		if err := rows.Scan(&website.ID, &website.SubscriptionID, &website.Type, &website.PrimaryDomain, &website.DocumentRoot, &website.Target, &website.RedirectCode, &website.PHPVersion, &website.Enabled, &website.SSLEnabled, &website.ForceHTTPS, &website.HSTS); err != nil {
			return nil, fmt.Errorf("scan website: %w", err)
		}
		websites = append(websites, website)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate websites: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close website rows: %w", err)
	}
	for index := range websites {
		aliases, err := repository.websiteAliases(ctx, websites[index].ID)
		if err != nil {
			return nil, err
		}
		websites[index].Aliases = aliases
	}
	return websites, nil
}

func (repository *Repository) websiteAliases(ctx context.Context, websiteID int64) ([]string, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT name FROM domains WHERE website_id = ? AND is_primary = 0 ORDER BY name`, websiteID)
	if err != nil {
		return nil, fmt.Errorf("list website aliases: %w", err)
	}
	defer rows.Close()
	var aliases []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scan website alias: %w", err)
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate website aliases: %w", err)
	}
	return aliases, nil
}

func (repository *Repository) CreateWebsite(ctx context.Context, website domain.Website) (int64, error) {
	transaction, err := repository.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin website insert: %w", err)
	}
	defer transaction.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := transaction.ExecContext(ctx, `INSERT INTO websites (subscription_id, type, document_root, target, redirect_code, php_version, enabled, ssl_enabled, force_https, hsts, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, website.SubscriptionID, website.Type, nullable(website.DocumentRoot), nullable(website.Target), nullableInt(website.RedirectCode), nullable(website.PHPVersion), website.Enabled, website.SSLEnabled, website.ForceHTTPS, website.HSTS, now, now)
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

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

// AddWebsiteAlias associates a globally unique alias with a website.
func (repository *Repository) AddWebsiteAlias(ctx context.Context, websiteID int64, alias string) error {
	_, err := repository.DB.ExecContext(ctx, `INSERT INTO domains (website_id, name, is_primary, created_at) VALUES (?, ?, 0, ?)`, websiteID, alias, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("add website alias %q: %w", alias, err)
	}
	return nil
}

// RemoveWebsiteAlias removes an alias only when it belongs to the website.
func (repository *Repository) RemoveWebsiteAlias(ctx context.Context, websiteID int64, alias string) error {
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM domains WHERE website_id = ? AND name = ? AND is_primary = 0`, websiteID, alias)
	if err != nil {
		return fmt.Errorf("remove website alias %q: %w", alias, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count website alias delete: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("website alias %q not found", alias)
	}
	return nil
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

// SetWebsiteEnabled records whether a website vhost is active.
func (repository *Repository) SetWebsiteEnabled(ctx context.Context, websiteID int64, enabled bool) error {
	result, err := repository.DB.ExecContext(ctx, `UPDATE websites SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, time.Now().UTC().Format(time.RFC3339), websiteID)
	if err != nil {
		return fmt.Errorf("set website %d enabled=%t: %w", websiteID, enabled, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count website update %d: %w", websiteID, err)
	}
	if rows != 1 {
		return fmt.Errorf("website %d not found", websiteID)
	}
	return nil
}
