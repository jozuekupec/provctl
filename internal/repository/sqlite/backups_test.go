package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestRepository_FinishBackupUpdatesOnlyRunningRecord(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	var subscriptionID int64
	if err := repository.DB.QueryRow("SELECT id FROM subscriptions WHERE name = 'acme'").Scan(&subscriptionID); err != nil {
		t.Fatal(err)
	}
	id, err := repository.CreateBackup(context.Background(), domain.Backup{SubscriptionID: subscriptionID, Path: "/backups/acme/next", Status: "running", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.FinishBackup(context.Background(), id, 42, "complete"); err != nil {
		t.Fatal(err)
	}
	backup, err := repository.BackupByID(context.Background(), subscriptionID, id)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Status != "complete" || backup.SizeBytes != 42 || backup.FinishedAt.IsZero() {
		t.Errorf("backup = %#v", backup)
	}
}

func TestRepository_BackupByIDAnyFindsRecordedBackup(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	var subscriptionID int64
	if err := repository.DB.QueryRow("SELECT id FROM subscriptions WHERE name = 'acme'").Scan(&subscriptionID); err != nil {
		t.Fatal(err)
	}
	id, err := repository.CreateBackup(context.Background(), domain.Backup{SubscriptionID: subscriptionID, Path: "/backups/acme/archive", Status: "running", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	backup, err := repository.BackupByIDAny(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Path != "/backups/acme/archive" || backup.SubscriptionID != subscriptionID {
		t.Errorf("backup = %#v", backup)
	}
}
