package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/system"
	"provctl/internal/system/fake"
)

type healthStore struct {
	subscriptions []domain.Subscription
	websites      map[int64][]domain.Website
}

func (store healthStore) ListSubscriptions(context.Context) ([]domain.Subscription, error) {
	return store.subscriptions, nil
}

func (store healthStore) SubscriptionByName(_ context.Context, name string) (domain.Subscription, error) {
	for _, subscription := range store.subscriptions {
		if subscription.Name == name {
			return subscription, nil
		}
	}
	return domain.Subscription{}, errors.New("subscription not found")
}

func (store healthStore) ListWebsites(_ context.Context, id int64) ([]domain.Website, error) {
	return store.websites[id], nil
}

type healthNetwork struct{ status int }

func (network healthNetwork) LookupHost(context.Context, string) ([]string, error) {
	return []string{"192.0.2.10"}, nil
}
func (network healthNetwork) ServerIPs() ([]string, error) { return []string{"192.0.2.10"}, nil }
func (network healthNetwork) Get(context.Context, string, string) (int, error) {
	return network.status, nil
}

type healthDatabase struct{ err error }

func (database healthDatabase) PingContext(context.Context) error { return database.err }

func TestHealthService_RunHealthyStaticWebsite(t *testing.T) {
	temporary := t.TempDir()
	documentRoot := filepath.Join(temporary, "public")
	enabled := filepath.Join(temporary, "enabled")
	if err := os.MkdirAll(documentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(enabled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(enabled, "provctl-acme-example.test.conf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	subscription := domain.Subscription{ID: 7, Name: "acme"}
	service := HealthService{
		FS: system.OSFS{}, Store: healthStore{subscriptions: []domain.Subscription{subscription}, websites: map[int64][]domain.Website{7: {{PrimaryDomain: "example.test", DocumentRoot: documentRoot, Enabled: true, Type: domain.WebsiteStatic}}}},
		Commands: &fake.Commander{}, Systemd: &fake.Systemd{IsActiveFunc: func(context.Context, string) (bool, error) { return true, nil }},
		Network: healthNetwork{status: 200}, Database: healthDatabase{}, Config: config.Config{Apache: config.Apache{Service: "apache2", SitesEnabled: enabled}},
	}
	checks, err := service.Run(context.Background(), "acme", "example.test")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if HasFailure(checks) {
		t.Errorf("Run() returned failed checks: %#v", checks)
	}
	if len(checks) != 7 {
		t.Errorf("len(checks) = %d, want 7", len(checks))
	}
}

func TestHealthService_RunReportsHTTPFailure(t *testing.T) {
	temporary := t.TempDir()
	documentRoot := filepath.Join(temporary, "public")
	enabled := filepath.Join(temporary, "enabled")
	if err := os.MkdirAll(documentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(enabled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(enabled, "provctl-acme-example.test.conf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	subscription := domain.Subscription{ID: 7, Name: "acme"}
	service := HealthService{
		FS: system.OSFS{}, Store: healthStore{subscriptions: []domain.Subscription{subscription}, websites: map[int64][]domain.Website{7: {{PrimaryDomain: "example.test", DocumentRoot: documentRoot, Enabled: true, Type: domain.WebsiteStatic}}}},
		Commands: &fake.Commander{}, Systemd: &fake.Systemd{IsActiveFunc: func(context.Context, string) (bool, error) { return true, nil }},
		Network: healthNetwork{status: 503}, Database: healthDatabase{}, Config: config.Config{Apache: config.Apache{Service: "apache2", SitesEnabled: enabled}},
	}
	checks, err := service.Run(context.Background(), "acme", "example.test")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !HasFailure(checks) {
		t.Errorf("Run() did not report the failed HTTP check: %#v", checks)
	}
}
