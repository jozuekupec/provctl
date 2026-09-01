package service

import (
	"context"
	"fmt"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
)

// BackupStore provides the read-only state needed before archive operations.
type BackupStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListBackups(context.Context, int64) ([]domain.Backup, error)
}

type BackupService struct{ Store BackupStore }

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

type BackupRuntime struct {
	Service    BackupService
	repository *sqlite.Repository
}

func NewReadOnlyBackupRuntime(ctx context.Context, _ config.Config) (*BackupRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &BackupRuntime{Service: BackupService{Store: repository}, repository: repository}, nil
}

func (runtime *BackupRuntime) Close() error { return runtime.repository.Close() }
