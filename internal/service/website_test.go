package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
)

type websiteLogFS struct {
	*subscriptionFS
	contents map[string][]byte
}

func (fs websiteLogFS) ReadFile(path string) ([]byte, error) {
	contents, ok := fs.contents[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return contents, nil
}

type websiteStore struct {
	subscription domain.Subscription
	websites     []domain.Website
}

func (store websiteStore) SubscriptionByName(context.Context, string) (domain.Subscription, error) {
	return store.subscription, nil
}
func (websiteStore) DomainExists(context.Context, string) (bool, error)           { return false, nil }
func (websiteStore) CreateWebsite(context.Context, domain.Website) (int64, error) { return 1, nil }
func (websiteStore) DeleteWebsite(context.Context, int64) error                   { return nil }
func (websiteStore) SetWebsiteEnabled(context.Context, int64, bool) error         { return nil }
func (websiteStore) SetWebsiteSSL(context.Context, int64, bool, bool) error       { return nil }
func (websiteStore) AddWebsiteAlias(context.Context, int64, string) error         { return nil }
func (websiteStore) RemoveWebsiteAlias(context.Context, int64, string) error      { return nil }
func (store websiteStore) ListWebsites(context.Context, int64) ([]domain.Website, error) {
	return store.websites, nil
}

type websiteApache struct{}

func (websiteApache) Apply(context.Context, string, []byte) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func (websiteApache) ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (websiteApache) SetVHostEnabled(context.Context, string, string, bool) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (websiteApache) RemoveVHost(context.Context, string, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

type websitePHPFPM struct{}

func (websitePHPFPM) ApplyPool(context.Context, PHPFPMVersion, string, []byte, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func (websitePHPFPM) RemovePool(context.Context, PHPFPMVersion, string) (func(context.Context) error, error) {
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

func TestWebsiteService_PrepareCreateProxyBuildsPlanWithoutChanges(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	service := WebsiteService{FS: fs, Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme", UnixUID: 5000, Home: "/vhosts/acme", Status: "active"}}, Apache: websiteApache{}, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", SitesEnabled: "/etc/apache2/sites-enabled", ProxyTimeout: 60}}}
	operation, err := service.PrepareCreateProxy(context.Background(), "acme", "proxy.example.test", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("PrepareCreateProxy() error = %v", err)
	}
	if len(fs.directories) != 0 {
		t.Error("PrepareCreateProxy() changed filesystem state")
	}
	if got, want := len(operation.Steps), 5; got != want {
		t.Errorf("plan steps = %d, want %d", got, want)
	}
}

func TestWebsiteService_PrepareCreateProxyRejectsExternalTarget(t *testing.T) {
	service := WebsiteService{Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme", UnixUID: 5000, Status: "active"}}, Apache: websiteApache{}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", SitesEnabled: "/etc/apache2/sites-enabled", ProxyTimeout: 60}}}
	_, err := service.PrepareCreateProxy(context.Background(), "acme", "proxy.example.test", "http://example.com:8080")
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("PrepareCreateProxy() error = %v, want allowlist validation error", err)
	}
}

func TestWebsiteService_PrepareCreateRedirectBuildsPlanWithoutChanges(t *testing.T) {
	fs := &subscriptionFS{directories: map[string]bool{}}
	service := WebsiteService{FS: fs, Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme", UnixUID: 5000, Status: "active"}}, Apache: websiteApache{}, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", SitesEnabled: "/etc/apache2/sites-enabled"}}}
	operation, err := service.PrepareCreateRedirect(context.Background(), "acme", "redirect.example.test", "https://target.example.test", 302)
	if err != nil {
		t.Fatalf("PrepareCreateRedirect() error = %v", err)
	}
	if len(fs.directories) != 0 {
		t.Error("PrepareCreateRedirect() changed filesystem state")
	}
	if got, want := len(operation.Steps), 5; got != want {
		t.Errorf("plan steps = %d, want %d", got, want)
	}
}

func TestWebsiteService_PrepareSetEnabledBuildsReversiblePlan(t *testing.T) {
	service := WebsiteService{Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme"}, websites: []domain.Website{{ID: 4, SubscriptionID: 1, PrimaryDomain: "example.test", Enabled: true}}}, Apache: websiteApache{}, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", SitesEnabled: "/etc/apache2/sites-enabled"}}}
	operation, err := service.PrepareSetEnabled(context.Background(), "acme", "example.test", false)
	if err != nil {
		t.Fatalf("PrepareSetEnabled() error = %v", err)
	}
	if got, want := len(operation.Steps), 2; got != want {
		t.Errorf("plan steps = %d, want %d", got, want)
	}
	if got, want := operation.Action, "website.set-enabled"; got != want {
		t.Errorf("action = %q, want %q", got, want)
	}
}

func TestWebsiteService_PrepareAliasBuildsReversiblePlan(t *testing.T) {
	service := WebsiteService{Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme"}, websites: []domain.Website{{ID: 4, SubscriptionID: 1, Type: domain.WebsiteStatic, PrimaryDomain: "example.test", DocumentRoot: "/vhosts/acme/sites/example.test/public"}}}, Apache: websiteApache{}, Executor: plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}, Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{SitesAvailable: "/etc/apache2/sites-available", ProxyTimeout: 60}}}
	operation, err := service.PrepareAlias(context.Background(), "acme", "example.test", "www.example.test", true)
	if err != nil {
		t.Fatalf("PrepareAlias() error = %v", err)
	}
	if got, want := len(operation.Steps), 2; got != want {
		t.Errorf("plan steps = %d, want %d", got, want)
	}
}

func TestWebsiteService_ReadLogsReturnsFinalLines(t *testing.T) {
	path := filepath.Join(meta.LogDir, "acme", "example.test", "access.log")
	fs := websiteLogFS{subscriptionFS: &subscriptionFS{directories: map[string]bool{}}, contents: map[string][]byte{path: []byte("one\ntwo\nthree\n")}}
	service := WebsiteService{FS: fs, Store: websiteStore{subscription: domain.Subscription{ID: 1, Name: "acme"}, websites: []domain.Website{{ID: 1, PrimaryDomain: "example.test"}}}}
	got, err := service.ReadLogs(context.Background(), "acme", "example.test", false, 2)
	if err != nil {
		t.Fatalf("ReadLogs() error = %v", err)
	}
	if want := "two\nthree\n"; got != want {
		t.Errorf("ReadLogs() = %q, want %q", got, want)
	}
}

func TestWebsiteService_renderHTTPVHost_RendersTLSForEveryWebsiteType(t *testing.T) {
	service := WebsiteService{Config: config.Config{Paths: config.Paths{ACMEChallenge: "/var/lib/provctl/acme-challenge"}, Apache: config.Apache{ProxyTimeout: 60}}}
	websites := []domain.Website{
		{Type: domain.WebsitePHPFPM, PrimaryDomain: "php.example.test", DocumentRoot: "/srv/php", SSLEnabled: true},
		{Type: domain.WebsiteStatic, PrimaryDomain: "static.example.test", DocumentRoot: "/srv/static", SSLEnabled: true},
		{Type: domain.WebsiteProxy, PrimaryDomain: "proxy.example.test", Target: "http://127.0.0.1:8080", SSLEnabled: true},
		{Type: domain.WebsiteRedirect, PrimaryDomain: "redirect.example.test", Target: "https://target.example.test", RedirectCode: 301, SSLEnabled: true},
	}
	for _, website := range websites {
		t.Run(string(website.Type), func(t *testing.T) {
			contents, err := service.renderHTTPVHost("acme", website)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(contents), "<VirtualHost *:80>") || !strings.Contains(string(contents), "<VirtualHost *:443>") || !strings.Contains(string(contents), "SSLCertificateFile /etc/letsencrypt/live/provctl-acme-"+website.PrimaryDomain+"/fullchain.pem") {
				t.Errorf("missing TLS vhost: %s", contents)
			}
		})
	}
}
