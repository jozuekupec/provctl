package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpen_AppliesInitialMigration(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	var version int
	if err := repository.DB.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 2 {
		t.Errorf("schema version = %d, want 2", version)
	}
}

func TestApplyMigrations_RejectsNewerSchema(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "newer.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO schema_migrations VALUES (3, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert migration: %v", err)
	}
	if err := ApplyMigrations(context.Background(), database); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("ApplyMigrations() error = %v, want ErrSchemaTooNew", err)
	}
}

func TestInspectSchema_ReadsCurrentVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provctl.db")
	repository, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	info, err := InspectSchema(context.Background(), path)
	if err != nil {
		t.Fatalf("InspectSchema() error = %v", err)
	}
	if info.Current != 2 || info.Latest != 2 {
		t.Errorf("InspectSchema() = %#v, want current and latest 2", info)
	}
}

func TestApplyMigrations_PreservesBackupsFromSchemaOne(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "schema-one.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	initial, err := migrationFiles.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(initial)); err != nil {
		t.Fatalf("create schema one: %v", err)
	}
	if _, err := database.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL); INSERT INTO schema_migrations VALUES (1, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO subscriptions (name, unix_user, unix_uid, home, status, php_max_children, php_memory_limit, php_upload_max, php_max_exec_time, ssh_access, created_at, updated_at) VALUES ('acme', 'acme', 5000, '/vhosts/acme', 'active', 10, '256M', '64M', 60, 'none', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'); INSERT INTO backups (subscription_id, path, status, started_at) VALUES (1, '/backups/acme/one', 'complete', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, database); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	var version int
	if err := database.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version = %d, %v", version, err)
	}
	if _, err := database.Exec(`DELETE FROM subscriptions WHERE id = 1`); err != nil {
		t.Fatalf("delete migrated subscription: %v", err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM backups WHERE subscription_id IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("orphaned backup count = %d, %v", count, err)
	}
}
