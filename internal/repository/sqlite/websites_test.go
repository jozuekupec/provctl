package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"provctl/internal/domain"
)

func TestRepository_CreateWebsite(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	subscription, err := repository.SubscriptionByName(context.Background(), "acme")
	if err != nil {
		t.Fatalf("SubscriptionByName() error = %v", err)
	}
	websiteID, err := repository.CreateWebsite(context.Background(), domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsitePHPFPM, PrimaryDomain: "example.test", DocumentRoot: "/vhosts/acme/sites/example.test/public", Enabled: true})
	if err != nil {
		t.Fatalf("CreateWebsite() error = %v", err)
	}
	if websiteID == 0 {
		t.Error("website ID = 0")
	}
	exists, err := repository.DomainExists(context.Background(), "example.test")
	if err != nil || !exists {
		t.Errorf("DomainExists() = %t, %v; want true, nil", exists, err)
	}
}
