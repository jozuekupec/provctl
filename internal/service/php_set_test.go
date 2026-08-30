package service

import (
	"context"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
)

type phpSetStore struct {
	updated []domain.Subscription
}

func (store *phpSetStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return domain.Subscription{}, nil
}
func (store *phpSetStore) ListWebsites(context.Context, int64) ([]domain.Website, error) {
	return nil, nil
}
func (store *phpSetStore) UpdatePHPSettings(_ context.Context, subscription domain.Subscription) error {
	store.updated = append(store.updated, subscription)
	return nil
}

type phpSetPools struct{ events []string }

func (pools *phpSetPools) ApplyPool(_ context.Context, version PHPFPMVersion, path string, _ []byte, _ string) (func(context.Context) error, error) {
	pools.events = append(pools.events, "apply "+version.Version+" "+path)
	return func(context.Context) error {
		pools.events = append(pools.events, "undo apply "+version.Version)
		return nil
	}, nil
}
func (pools *phpSetPools) RemovePool(_ context.Context, version PHPFPMVersion, path string) (func(context.Context) error, error) {
	pools.events = append(pools.events, "remove "+version.Version+" "+path)
	return func(context.Context) error {
		pools.events = append(pools.events, "undo remove "+version.Version)
		return nil
	}, nil
}

type phpSetApache struct{ events []string }

func (apache *phpSetApache) Apply(_ context.Context, path string, _ []byte) (func(context.Context) error, error) {
	apache.events = append(apache.events, "apply "+path)
	return func(context.Context) error { apache.events = append(apache.events, "undo "+path); return nil }, nil
}
func (*phpSetApache) ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error) {
	return nil, nil
}
func (*phpSetApache) SetVHostEnabled(context.Context, string, string, bool) (func(context.Context) error, error) {
	return nil, nil
}
func (*phpSetApache) RemoveVHost(context.Context, string, string) (func(context.Context) error, error) {
	return nil, nil
}

func TestPHPService_SetPlanOrdersTransitionAndRecordsSettings(t *testing.T) {
	store, pools, apache := &phpSetStore{}, &phpSetPools{}, &phpSetApache{}
	service := PHPService{Store: store, PHPFPM: pools, Apache: apache, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/acme"}, Apache: config.Apache{SitesAvailable: "/available", ProxyTimeout: 60}}}
	previous := domain.Subscription{ID: 1, Name: "acme", Home: "/vhosts/acme", PHPVersion: "8.3", PHPMaxChildren: 10, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60}
	desired := previous
	desired.PHPVersion, desired.PHPMaxChildren = "8.4", 20
	websites := []domain.Website{{Type: domain.WebsitePHPFPM, PrimaryDomain: "app.example.test", DocumentRoot: "/vhosts/acme/sites/app.example.test/public"}, {Type: domain.WebsiteStatic, PrimaryDomain: "static.example.test", DocumentRoot: "/vhosts/acme/sites/static.example.test/public"}}
	operation, err := service.setPlan(previous, desired, websites, PHPFPMVersion{Version: "8.3", Binary: "old", Service: "old.service"}, PHPFPMVersion{Version: "8.4", Binary: "new", Service: "new.service"}, []byte("pool"))
	if err != nil {
		t.Fatalf("setPlan() error = %v", err)
	}
	if got, want := len(operation.Steps), 5; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}
	if got, want := operation.Steps[1].Name, "regenerate Apache vhost app.example.test"; got != want {
		t.Errorf("second step = %q, want %q", got, want)
	}
	if _, err := service.Executor.Run(context.Background(), operation); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := strings.Join(pools.events, "\n"), "apply 8.4 /etc/php/8.4/fpm/pool.d/provctl-acme.conf\nremove 8.3 /etc/php/8.3/fpm/pool.d/provctl-acme.conf"; got != want {
		t.Errorf("pool events (-want +got):\n%s", got)
	}
	if got, want := strings.Join(apache.events, "\n"), "apply /available/provctl-acme-app.example.test.conf\napply /available/provctl-acme-static.example.test.conf"; got != want {
		t.Errorf("Apache events (-want +got):\n%s", got)
	}
	if len(store.updated) != 1 || store.updated[0].PHPVersion != "8.4" || store.updated[0].PHPMaxChildren != 20 {
		t.Errorf("recorded settings = %#v", store.updated)
	}
}

func TestPHPService_PrepareSetRejectsInvalidVersionBeforeChanges(t *testing.T) {
	service := PHPService{Store: &phpSetStore{}, PHPFPM: &phpSetPools{}, Apache: &phpSetApache{}}
	_, err := service.PrepareSet(context.Background(), "acme", PHPSetOptions{Version: "latest"})
	if err == nil || !strings.Contains(err.Error(), "major.minor") {
		t.Fatalf("PrepareSet() error = %v, want version validation error", err)
	}
}

func TestPHPService_SetPlanKeepsPoolWhenOnlyLimitsChange(t *testing.T) {
	service := PHPService{Apache: &phpSetApache{}, PHPFPM: &phpSetPools{}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/acme"}, Apache: config.Apache{SitesAvailable: "/available", ProxyTimeout: 60}}}
	subscription := domain.Subscription{ID: 1, Name: "acme", Home: "/vhosts/acme", PHPVersion: "8.4", PHPMaxChildren: 12, PHPMemoryLimit: "256M", PHPUploadMax: "64M", PHPMaxExecTime: 60}
	version := PHPFPMVersion{Version: "8.4", Binary: "php-fpm8.4", Service: "php8.4-fpm.service"}
	operation, err := service.setPlan(subscription, subscription, []domain.Website{{Type: domain.WebsitePHPFPM, PrimaryDomain: "app.example.test", DocumentRoot: "/vhosts/acme/sites/app.example.test/public"}}, version, version, []byte("pool"))
	if err != nil {
		t.Fatalf("setPlan() error = %v", err)
	}
	for _, step := range operation.Steps {
		if step.Name == "remove previous PHP-FPM pool" {
			t.Fatal("same-version plan must not remove its active pool")
		}
	}
}
