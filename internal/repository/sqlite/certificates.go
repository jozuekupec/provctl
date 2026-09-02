package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"provctl/internal/domain"
)

func (repository *Repository) CreateCertificate(ctx context.Context, certificate domain.Certificate) (int64, error) {
	sans, err := json.Marshal(certificate.SANs)
	if err != nil {
		return 0, fmt.Errorf("encode certificate SANs: %w", err)
	}
	result, err := repository.DB.ExecContext(ctx, `INSERT INTO certificates (subscription_id, lineage, primary_domain, sans, issuer, not_before, not_after, last_checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, certificate.SubscriptionID, certificate.Lineage, certificate.PrimaryDomain, string(sans), nullable(certificate.Issuer), nullableTime(certificate.NotBefore), nullableTime(certificate.NotAfter), nullableTime(certificate.LastCheckedAt))
	if err != nil {
		return 0, fmt.Errorf("insert certificate %q: %w", certificate.Lineage, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read certificate ID: %w", err)
	}
	return id, nil
}

// ListCertificates returns the persisted certificate metadata for one subscription.
// Archive restore deliberately does not reinstate these certificates, but records
// them in the backup manifest so the operator can reissue the expected lineages.
func (repository *Repository) ListCertificates(ctx context.Context, subscriptionID int64) ([]domain.Certificate, error) {
	rows, err := repository.DB.QueryContext(ctx, `SELECT id, subscription_id, lineage, primary_domain, sans, COALESCE(issuer, ''), COALESCE(not_before, ''), COALESCE(not_after, ''), COALESCE(last_checked_at, '') FROM certificates WHERE subscription_id = ? ORDER BY lineage`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	defer rows.Close()
	var certificates []domain.Certificate
	for rows.Next() {
		var certificate domain.Certificate
		var sans, notBefore, notAfter, checked string
		if err := rows.Scan(&certificate.ID, &certificate.SubscriptionID, &certificate.Lineage, &certificate.PrimaryDomain, &sans, &certificate.Issuer, &notBefore, &notAfter, &checked); err != nil {
			return nil, fmt.Errorf("scan certificate: %w", err)
		}
		if err := json.Unmarshal([]byte(sans), &certificate.SANs); err != nil {
			return nil, fmt.Errorf("decode certificate SANs: %w", err)
		}
		var parseErr error
		if certificate.NotBefore, parseErr = parseNullableTime(notBefore); parseErr != nil {
			return nil, parseErr
		}
		if certificate.NotAfter, parseErr = parseNullableTime(notAfter); parseErr != nil {
			return nil, parseErr
		}
		if certificate.LastCheckedAt, parseErr = parseNullableTime(checked); parseErr != nil {
			return nil, parseErr
		}
		certificates = append(certificates, certificate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate certificates: %w", err)
	}
	return certificates, nil
}

// DeleteCertificatesBySubscription removes certificate metadata when its owning
// subscription is permanently removed. Certificate files are not touched here.
func (repository *Repository) DeleteCertificatesBySubscription(ctx context.Context, subscriptionID int64) error {
	if _, err := repository.DB.ExecContext(ctx, `DELETE FROM certificates WHERE subscription_id = ?`, subscriptionID); err != nil {
		return fmt.Errorf("delete certificates for subscription %d: %w", subscriptionID, err)
	}
	return nil
}

func (repository *Repository) CertificateByLineage(ctx context.Context, lineage string) (domain.Certificate, error) {
	row := repository.DB.QueryRowContext(ctx, `SELECT id, subscription_id, lineage, primary_domain, sans, COALESCE(issuer, ''), COALESCE(not_before, ''), COALESCE(not_after, ''), COALESCE(last_checked_at, '') FROM certificates WHERE lineage = ?`, lineage)
	return scanCertificate(row)
}

func (repository *Repository) UpdateCertificateNotAfter(ctx context.Context, lineage string, notAfter time.Time) (bool, error) {
	result, err := repository.DB.ExecContext(ctx, `UPDATE certificates SET not_after = ?, last_checked_at = ? WHERE lineage = ?`, notAfter.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), lineage)
	if err != nil {
		return false, fmt.Errorf("update certificate %q: %w", lineage, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count certificate update: %w", err)
	}
	return rows == 1, nil
}

func scanCertificate(row *sql.Row) (domain.Certificate, error) {
	var certificate domain.Certificate
	var sans, notBefore, notAfter, checked string
	if err := row.Scan(&certificate.ID, &certificate.SubscriptionID, &certificate.Lineage, &certificate.PrimaryDomain, &sans, &certificate.Issuer, &notBefore, &notAfter, &checked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Certificate{}, fmt.Errorf("certificate not found")
		}
		return domain.Certificate{}, fmt.Errorf("scan certificate: %w", err)
	}
	if err := json.Unmarshal([]byte(sans), &certificate.SANs); err != nil {
		return domain.Certificate{}, fmt.Errorf("decode certificate SANs: %w", err)
	}
	var err error
	if certificate.NotBefore, err = parseNullableTime(notBefore); err != nil {
		return domain.Certificate{}, err
	}
	if certificate.NotAfter, err = parseNullableTime(notAfter); err != nil {
		return domain.Certificate{}, err
	}
	if certificate.LastCheckedAt, err = parseNullableTime(checked); err != nil {
		return domain.Certificate{}, err
	}
	return certificate, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func parseNullableTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse certificate timestamp: %w", err)
	}
	return parsed, nil
}
