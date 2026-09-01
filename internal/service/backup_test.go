package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/system/fake"
)

type backupStore struct {
	subscription domain.Subscription
	backups      []domain.Backup
}

func (store backupStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return store.subscription, nil
}
func (store backupStore) ListBackups(context.Context, int64) ([]domain.Backup, error) {
	return store.backups, nil
}
func (store backupStore) BackupByID(_ context.Context, _ int64, id int64) (domain.Backup, error) {
	for _, backup := range store.backups {
		if backup.ID == id {
			return backup, nil
		}
	}
	return domain.Backup{}, context.Canceled
}
func (backupStore) CreateBackup(context.Context, domain.Backup) (int64, error)      { return 1, nil }
func (backupStore) FinishBackup(context.Context, int64, int64, string) error        { return nil }
func (backupStore) ListDatabases(context.Context, int64) ([]domain.Database, error) { return nil, nil }

func TestBackupService_ListForSubscriptionRejectsInvalidName(t *testing.T) {
	_, err := (BackupService{Store: backupStore{}}).ListForSubscription(context.Background(), "BAD")
	if err == nil || !strings.Contains(err.Error(), "subscription name") {
		t.Fatalf("error = %v", err)
	}
}

func TestBackupService_InspectReadsMatchingManifest(t *testing.T) {
	manifest := []byte(`{"format_version":1,"created_at":"2026-01-01T00:00:00Z","subscription":{"name":"acme"}}`)
	service := BackupService{
		Store: backupStore{subscription: domain.Subscription{ID: 1, Name: "acme"}, backups: []domain.Backup{{ID: 4, Path: "/backups/acme/2026-01-01"}}},
		FS: &fake.FS{ReadFileFunc: func(path string) ([]byte, error) {
			switch path {
			case "/backups/acme/2026-01-01/metadata.json":
				return manifest, nil
			case "/backups/acme/2026-01-01/SHA256SUMS":
				return []byte(fmt.Sprintf("%x  metadata.json\n", sha256.Sum256(manifest))), nil
			default:
				t.Fatalf("path = %q", path)
			}
			return nil, nil
		}},
		Config: config.Config{Paths: config.Paths{Backups: "/backups"}},
	}
	metadata, err := service.Inspect(context.Background(), "acme", 4)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.FormatVersion != 1 || metadata.Subscription.Name != "acme" {
		t.Errorf("metadata = %#v", metadata)
	}
}

func TestBackupService_CreateRejectsBackupQuotaBeforeSystemChanges(t *testing.T) {
	service := BackupService{Store: backupStore{subscription: domain.Subscription{ID: 1, Name: "acme", QuotaBackups: 1}, backups: []domain.Backup{{ID: 1}}}}
	_, err := service.Create(context.Background(), "acme")
	if err == nil || !strings.Contains(err.Error(), "backup quota") {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestBackupService_ExtractArchiveUsesExplicitTarArguments(t *testing.T) {
	fs := &fake.FS{MkdirAllFunc: func(string, os.FileMode) error { return nil }, RemoveAllFunc: func(string) error { return nil }}
	commands := &fake.Commander{}
	service := BackupService{FS: fs, Commands: commands}
	if err := service.extractArchive(context.Background(), "/backups/acme/files.tar.zst", "/vhosts/.restore-acme"); err != nil {
		t.Fatal(err)
	}
	if len(commands.Calls) != 1 || commands.Calls[0].Name != "/usr/bin/tar" || strings.Contains(strings.Join(commands.Calls[0].Args, " "), "sh -c") {
		t.Errorf("calls = %#v", commands.Calls)
	}
}

func TestBackupService_PromoteStagingRejectsExistingTarget(t *testing.T) {
	fs := &fake.FS{StatFunc: func(string) (os.FileInfo, error) { return subscriptionInfo{}, nil }}
	service := BackupService{FS: fs}
	err := service.promoteStaging("/vhosts/.restore-acme", "/vhosts/acme")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("promoteStaging() error = %v", err)
	}
}
