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
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
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
	if _, err := database.Exec(`INSERT INTO schema_migrations VALUES (2, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert migration: %v", err)
	}
	if err := ApplyMigrations(context.Background(), database); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("ApplyMigrations() error = %v, want ErrSchemaTooNew", err)
	}
}
