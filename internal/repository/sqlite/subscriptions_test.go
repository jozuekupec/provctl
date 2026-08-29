package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"provctl/internal/domain"
)

func TestRepository_CreateAndDeleteSubscription(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	subscription := domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}
	if err := repository.CreateSubscription(context.Background(), subscription); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	exists, err := repository.SubscriptionExists(context.Background(), "acme")
	if err != nil || !exists {
		t.Fatalf("SubscriptionExists() = %v, %v; want true, nil", exists, err)
	}
	if err := repository.DeleteSubscription(context.Background(), "acme"); err != nil {
		t.Fatalf("DeleteSubscription() error = %v", err)
	}
}
