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

func TestRepository_ListAndFindSubscriptions(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	for _, subscription := range []domain.Subscription{
		{Name: "beta", UnixUser: "beta", UnixUID: 5001, Home: "/vhosts/beta", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"},
		{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"},
	} {
		if err := repository.CreateSubscription(context.Background(), subscription); err != nil {
			t.Fatalf("CreateSubscription(%q) error = %v", subscription.Name, err)
		}
	}
	subscriptions, err := repository.ListSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("ListSubscriptions() error = %v", err)
	}
	if got, want := len(subscriptions), 2; got != want {
		t.Fatalf("subscription count = %d, want %d", got, want)
	}
	if got, want := subscriptions[0].Name, "acme"; got != want {
		t.Errorf("first subscription = %q, want %q", got, want)
	}
	found, err := repository.SubscriptionByName(context.Background(), "beta")
	if err != nil {
		t.Fatalf("SubscriptionByName() error = %v", err)
	}
	if got, want := found.UnixUID, 5001; got != want {
		t.Errorf("UnixUID = %d, want %d", got, want)
	}
}
