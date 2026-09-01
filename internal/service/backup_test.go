package service

import (
	"context"
	"strings"
	"testing"

	"provctl/internal/domain"
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

func TestBackupService_ListForSubscriptionRejectsInvalidName(t *testing.T) {
	_, err := (BackupService{Store: backupStore{}}).ListForSubscription(context.Background(), "BAD")
	if err == nil || !strings.Contains(err.Error(), "subscription name") {
		t.Fatalf("error = %v", err)
	}
}
