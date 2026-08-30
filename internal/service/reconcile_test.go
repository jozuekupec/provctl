package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
	"provctl/internal/system"
)

type reconcileStore struct {
	subscriptions []domain.Subscription
	websites      map[int64][]domain.Website
}

func (store reconcileStore) ListSubscriptions(context.Context) ([]domain.Subscription, error) {
	return store.subscriptions, nil
}

func (store reconcileStore) ListWebsites(_ context.Context, subscriptionID int64) ([]domain.Website, error) {
	return store.websites[subscriptionID], nil
}

type reconcileApache struct {
	applied []string
	enabled []string
}

func (apache *reconcileApache) Apply(_ context.Context, path string, _ []byte) (func(context.Context) error, error) {
	apache.applied = append(apache.applied, path)
	return func(context.Context) error { return nil }, nil
}
func (*reconcileApache) ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error) {
	return nil, nil
}
func (apache *reconcileApache) SetVHostEnabled(_ context.Context, _ string, path string, enabled bool) (func(context.Context) error, error) {
	apache.enabled = append(apache.enabled, path+":"+map[bool]string{true: "true", false: "false"}[enabled])
	return func(context.Context) error { return nil }, nil
}
func (*reconcileApache) RemoveVHost(context.Context, string, string) (func(context.Context) error, error) {
	return nil, nil
}

func TestReconcileService_InspectReportsVhostAndEnabledStateDrift(t *testing.T) {
	directory := t.TempDir()
	available, enabled := filepath.Join(directory, "available"), filepath.Join(directory, "enabled")
	if err := os.MkdirAll(available, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(enabled, 0o755); err != nil {
		t.Fatal(err)
	}
	store := reconcileStore{subscriptions: []domain.Subscription{{ID: 4, Name: "acme"}}, websites: map[int64][]domain.Website{4: {{ID: 8, Type: domain.WebsiteStatic, PrimaryDomain: "example.test", DocumentRoot: "/vhosts/acme/sites/example.test/public", Enabled: true}}}}
	service := ReconcileService{FS: system.OSFS{}, Store: store, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: available, SitesEnabled: enabled, ProxyTimeout: 60}}}

	drifts, err := service.Inspect(context.Background(), "acme")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if got, want := len(drifts), 2; got != want {
		t.Fatalf("drifts = %d, want %d", got, want)
	}
	if !strings.Contains(UnifiedDiff(drifts[0].Path, drifts[0].Current, drifts[0].Expected), "+<VirtualHost *:80>") {
		t.Error("UnifiedDiff() does not contain generated vhost content")
	}
}

func TestReconcileService_PrepareBuildsOnlyRequiredSteps(t *testing.T) {
	directory := t.TempDir()
	available, enabled := filepath.Join(directory, "available"), filepath.Join(directory, "enabled")
	if err := os.MkdirAll(available, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(enabled, 0o755); err != nil {
		t.Fatal(err)
	}
	store := reconcileStore{subscriptions: []domain.Subscription{{ID: 4, Name: "acme"}}, websites: map[int64][]domain.Website{4: {{ID: 8, Type: domain.WebsiteStatic, PrimaryDomain: "example.test", DocumentRoot: "/vhosts/acme/sites/example.test/public", Enabled: false}}}}
	apache := &reconcileApache{}
	service := ReconcileService{FS: system.OSFS{}, Store: store, Apache: apache, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: available, SitesEnabled: enabled, ProxyTimeout: 60}}}

	operation, drifts, err := service.Prepare(context.Background(), "acme")
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if got, want := len(drifts), 1; got != want {
		t.Fatalf("drifts = %d, want %d", got, want)
	}
	if got, want := len(operation.Steps), 1; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}
	if _, err := service.Executor.Run(context.Background(), operation); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := apache.applied, []string{filepath.Join(available, "provctl-acme-example.test.conf")}; !equalStrings(got, want) {
		t.Errorf("applied = %v, want %v", got, want)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
