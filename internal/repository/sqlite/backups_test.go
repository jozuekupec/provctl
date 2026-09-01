package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"provctl/internal/domain"
)

func TestRepository_ListBackups_OrdersNewestFirst(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := repository.DB.QueryRow("SELECT id FROM subscriptions WHERE name = 'acme'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.DB.Exec(`INSERT INTO backups (subscription_id, path, size_bytes, status, started_at, finished_at) VALUES (?, ?, ?, 'complete', ?, ?)`, id, "/backups/old", 10, "2026-01-01T00:00:00Z", "2026-01-01T00:01:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.DB.Exec(`INSERT INTO backups (subscription_id, path, status, started_at) VALUES (?, ?, 'running', ?)`, id, "/backups/new", "2026-01-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	backups, err := repository.ListBackups(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(backups), 2; got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
	if got, want := backups[0].Path, "/backups/new"; got != want {
		t.Errorf("newest path = %q, want %q", got, want)
	}
	if got, want := backups[1].SizeBytes, int64(10); got != want {
		t.Errorf("size = %d, want %d", got, want)
	}
}
