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

func TestRepository_ListWebsitesIncludesProxyTarget(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	subscription, err := repository.SubscriptionByName(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateWebsite(context.Background(), domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteProxy, PrimaryDomain: "proxy.example.test", Target: "http://127.0.0.1:8080", Enabled: true}); err != nil {
		t.Fatalf("CreateWebsite() error = %v", err)
	}
	websites, err := repository.ListWebsites(context.Background(), subscription.ID)
	if err != nil {
		t.Fatalf("ListWebsites() error = %v", err)
	}
	if len(websites) != 1 {
		t.Fatalf("ListWebsites() returned %d websites, want 1", len(websites))
	}
	if target := websites[0].Target; target != "http://127.0.0.1:8080" {
		t.Errorf("Target = %q", target)
	}
	if err := repository.AddWebsiteAlias(context.Background(), websites[0].ID, "www.proxy.example.test"); err != nil {
		t.Fatalf("AddWebsiteAlias() error = %v", err)
	}
	websites, err = repository.ListWebsites(context.Background(), subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(websites[0].Aliases), 1; got != want || websites[0].Aliases[0] != "www.proxy.example.test" {
		t.Errorf("Aliases = %#v", websites[0].Aliases)
	}
}

func TestRepository_SetWebsiteEnabledUpdatesPersistedState(t *testing.T) {
	repository, err := Open(context.Background(), filepath.Join(t.TempDir(), "provctl.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer repository.Close()
	if err := repository.CreateSubscription(context.Background(), domain.Subscription{Name: "acme", UnixUser: "acme", UnixUID: 5000, Home: "/vhosts/acme", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60, SSHAccess: "none"}); err != nil {
		t.Fatal(err)
	}
	subscription, err := repository.SubscriptionByName(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	websiteID, err := repository.CreateWebsite(context.Background(), domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteStatic, PrimaryDomain: "example.test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SetWebsiteEnabled(context.Background(), websiteID, false); err != nil {
		t.Fatalf("SetWebsiteEnabled() error = %v", err)
	}
	websites, err := repository.ListWebsites(context.Background(), subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(websites) != 1 || websites[0].Enabled {
		t.Errorf("ListWebsites() = %#v, want one disabled website", websites)
	}
}
