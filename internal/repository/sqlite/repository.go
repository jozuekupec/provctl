// Package sqlite provides SQLite persistence and schema migrations.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrSchemaTooNew = errors.New("database schema is newer than this binary supports")

type Repository struct {
	DB *sql.DB
}

func Open(ctx context.Context, databasePath string) (*Repository, error) {
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}
	if err := configure(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := ApplyMigrations(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Repository{DB: database}, nil
}

func (repository *Repository) Close() error { return repository.DB.Close() }

func configure(ctx context.Context, database *sql.DB) error {
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure SQLite (%s): %w", statement, err)
		}
	}
	return nil
}

func ApplyMigrations(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        applied_at TEXT NOT NULL
    )`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	current, err := currentVersion(ctx, database)
	if err != nil {
		return err
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return errors.New("no embedded SQLite migrations")
	}
	latest := migrations[len(migrations)-1].version
	if current > latest {
		return fmt.Errorf("%w: found %d, latest supported is %d", ErrSchemaTooNew, current, latest)
	}
	for _, migration := range migrations {
		if migration.version <= current {
			continue
		}
		if err := applyMigration(ctx, database, migration); err != nil {
			return err
		}
	}
	return nil
}

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".sql" {
			continue
		}
		versionText, _, found := strings.Cut(entry.Name(), "_")
		if !found {
			return nil, fmt.Errorf("invalid migration name %q", entry.Name())
		}
		version, err := strconv.Atoi(versionText)
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}
		contents, err := migrationFiles.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(contents)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

func currentVersion(ctx context.Context, database *sql.DB) (int, error) {
	var version sql.NullInt64
	if err := database.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("read database schema version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

func applyMigration(ctx context.Context, database *sql.DB, migration migration) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.name, err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, migration.sql); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.name, err)
	}
	if _, err := transaction.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		migration.version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.name, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.name, err)
	}
	return nil
}
