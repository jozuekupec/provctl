package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// BackupStore provides the read-only state needed before archive operations.
type BackupStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListBackups(context.Context, int64) ([]domain.Backup, error)
	BackupByID(context.Context, int64, int64) (domain.Backup, error)
}

type BackupService struct {
	Store  BackupStore
	FS     system.FS
	Config config.Config
}

func (service BackupService) ListForSubscription(ctx context.Context, name string) ([]domain.Backup, error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return nil, err
	}
	if service.Store == nil {
		return nil, fmt.Errorf("backup store is required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return nil, err
	}
	backups, err := service.Store.ListBackups(ctx, subscription.ID)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	return backups, nil
}

func (service BackupService) Inspect(ctx context.Context, name string, id int64) (domain.BackupMetadata, error) {
	if id < 1 {
		return domain.BackupMetadata{}, fmt.Errorf("backup ID must be positive")
	}
	if service.FS == nil {
		return domain.BackupMetadata{}, fmt.Errorf("filesystem is required")
	}
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return domain.BackupMetadata{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return domain.BackupMetadata{}, err
	}
	backup, err := service.Store.BackupByID(ctx, subscription.ID, id)
	if err != nil {
		return domain.BackupMetadata{}, err
	}
	expectedRoot := filepath.Join(service.Config.Paths.Backups, name)
	relative, err := filepath.Rel(expectedRoot, backup.Path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return domain.BackupMetadata{}, fmt.Errorf("backup path is outside subscription backup directory")
	}
	contents, err := service.FS.ReadFile(filepath.Join(backup.Path, "metadata.json"))
	if err != nil {
		return domain.BackupMetadata{}, fmt.Errorf("read backup metadata: %w", err)
	}
	var metadata domain.BackupMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return domain.BackupMetadata{}, fmt.Errorf("decode backup metadata: %w", err)
	}
	if metadata.FormatVersion != 1 || metadata.Subscription.Name != name {
		return domain.BackupMetadata{}, fmt.Errorf("backup metadata does not match supported format and subscription")
	}
	if err := service.verifyChecksums(backup.Path); err != nil {
		return domain.BackupMetadata{}, err
	}
	return metadata, nil
}

func (service BackupService) verifyChecksums(backupPath string) error {
	contents, err := service.FS.ReadFile(filepath.Join(backupPath, "SHA256SUMS"))
	if err != nil {
		return fmt.Errorf("read backup checksums: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return fmt.Errorf("invalid backup checksum entry")
		}
		name := filepath.Clean(fields[1])
		if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid backup checksum path")
		}
		file, err := service.FS.ReadFile(filepath.Join(backupPath, name))
		if err != nil {
			return fmt.Errorf("read backup file %q: %w", name, err)
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(file))
		if actual != strings.ToLower(fields[0]) {
			return fmt.Errorf("backup checksum mismatch for %q", name)
		}
	}
	return nil
}

type BackupRuntime struct {
	Service    BackupService
	repository *sqlite.Repository
}

func NewReadOnlyBackupRuntime(ctx context.Context, cfg config.Config) (*BackupRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &BackupRuntime{Service: BackupService{Store: repository, FS: system.OSFS{}, Config: cfg}, repository: repository}, nil
}

func (runtime *BackupRuntime) Close() error { return runtime.repository.Close() }
