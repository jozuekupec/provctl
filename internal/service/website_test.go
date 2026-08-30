package service

import (
	"context"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
)

type websiteStore struct{ subscription domain.Subscription }

func (store websiteStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return store.subscription, nil
}
func (websiteStore) DomainExists(context.Context, string) (bool, error)            { return false, nil }
func (websiteStore) CreateWebsite(context.Context, domain.Website) (int64, error)  { return 1, nil }
func (websiteStore) DeleteWebsite(context.Context, int64) error                    { return nil }
func (websiteStore) ListWebsites(context.Context, int64) ([]domain.Website, error) { return nil, nil }

type websiteApache struct{}

func (websiteApache) ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

type websitePHPFPM struct{}

func (websitePHPFPM) ApplyPool(context.Context, PHPFPMVersion, string, []byte, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func TestWebsiteService_PrepareCreatePHPFPMBuildsPlanWithoutChanges(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	service := WebsiteService{
		FS: fs, Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "active", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60}}, Apache: websiteApache{}, PHPFPM: websitePHPFPM{}, Version: PHPFPMVersion{Version: "7.9", Binary: "/usr/sbin/php-fpm7.9", Service: "php7.9-fpm.service"},
		Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}},
		Config:   config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", SitesEnabled: "/etc/apache2/sites-enabled", ProxyTimeout: 60}},
	}
	operation, err := service.PrepareCreatePHPFPM(context.Background(), "acme", "example.test")
	if err != nil {
		t.Fatalf("PrepareCreatePHPFPM() error = %v", err)
	}
	if len(fs.directories) != 0 {
		t.Error("PrepareCreatePHPFPM() changed filesystem state")
	}
	if got, want := operation.Action, "website.create"; got != want {
		t.Errorf("action = %q, want %q", got, want)
	}
	if got, want := len(operation.Steps), 12; got != want {
		t.Errorf("plan steps = %d, want %d", got, want)
	}
}

func TestWebsiteService_PrepareCreateStaticBuildsPlanWithoutChanges(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	service := WebsiteService{FS: fs, Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "active"}}, Apache: websiteApache{}, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", SitesEnabled: "/etc/apache2/sites-enabled", ProxyTimeout: 60}}}
	operation, err := service.PrepareCreateStatic(context.Background(), "acme", "static.example.test")
	if err != nil {
		t.Fatalf("PrepareCreateStatic() error = %v", err)
	}
	if len(fs.directories) != 0 {
		t.Error("PrepareCreateStatic() changed filesystem state")
	}
	if got, want := len(operation.Steps), 7; got != want {
		t.Errorf("plan steps = %d, want %d", got, want)
	}
}
