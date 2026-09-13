package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"provctl/internal/config"
	"provctl/internal/system"
	"provctl/internal/system/fake"
)

type certificateStore struct {
	updated  string
	notAfter time.Time
	lineage  string
}

func (store *certificateStore) UpdateCertificateNotAfter(_ context.Context, lineage string, notAfter time.Time) (bool, error) {
	store.updated, store.notAfter = lineage, notAfter
	return true, nil
}
func (store *certificateStore) WebsiteCertificateName(context.Context, string, string) (string, error) {
	return store.lineage, nil
}

type certificateSystemd struct {
	reloaded string
	err      error
}

type certificateNetwork struct {
	addresses map[string][]string
	serverIPs []string
	status    int
	requests  *[]string
}

func (network certificateNetwork) LookupHost(_ context.Context, host string) ([]string, error) {
	return network.addresses[host], nil
}
func (network certificateNetwork) ServerIPs() ([]string, error) { return network.serverIPs, nil }
func (network certificateNetwork) Get(_ context.Context, target string) (int, error) {
	if network.requests != nil {
		*network.requests = append(*network.requests, target)
	}
	return network.status, nil
}

func (systemd *certificateSystemd) Reload(_ context.Context, unit string) error {
	systemd.reloaded = unit
	return systemd.err
}
func (systemd *certificateSystemd) Restart(context.Context, string) error          { return nil }
func (systemd *certificateSystemd) Start(context.Context, string) error            { return nil }
func (systemd *certificateSystemd) Stop(context.Context, string) error             { return nil }
func (systemd *certificateSystemd) IsActive(context.Context, string) (bool, error) { return true, nil }
func (systemd *certificateSystemd) Enable(context.Context, string) error           { return nil }
func (systemd *certificateSystemd) Disable(context.Context, string) error          { return nil }

func TestCertificateService_StatusReadsLiveCertificate(t *testing.T) {
	live := filepath.Join(t.TempDir(), "live")
	path := filepath.Join(live, "provctl-acme-example.test")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "cert.pem"), []byte("certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	commander := &fake.Commander{Result: system.Result{Stdout: "notAfter=Sep  1 12:00:00 2026 GMT\n"}}
	service := CertificateService{FS: system.OSFS{}, Commands: commander, LiveDir: live}
	status, err := service.Status(context.Background(), "acme", "example.test")
	if err != nil {
		t.Fatal(err)
	}
	if status.Lineage != "provctl-acme-example.test" || !status.NotAfter.Equal(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("Status() = %#v", status)
	}
	if got := commander.Calls[0]; got.Name != "/usr/bin/openssl" || got.Args[2] != filepath.Join(path, "cert.pem") {
		t.Errorf("openssl call = %#v", got)
	}
}

func TestCertificateService_StatusUsesWebsiteCertificateName(t *testing.T) {
	live := filepath.Join(t.TempDir(), "live")
	path := filepath.Join(live, "provctl-site-42")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "cert.pem"), []byte("certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	commander := &fake.Commander{Result: system.Result{Stdout: "notAfter=Sep  1 12:00:00 2026 GMT\n"}}
	service := CertificateService{Store: &certificateStore{lineage: "provctl-site-42"}, FS: system.OSFS{}, Commands: commander, LiveDir: live}
	status, err := service.Status(context.Background(), "acme", "example.test")
	if err != nil {
		t.Fatal(err)
	}
	if status.Lineage != "provctl-site-42" {
		t.Errorf("Status().Lineage = %q, want stable website lineage", status.Lineage)
	}
	if got := commander.Calls[0].Args[2]; got != filepath.Join(path, "cert.pem") {
		t.Errorf("openssl certificate path = %q", got)
	}
}

func TestCertificateService_DeployHookRejectsOutsideLiveDir(t *testing.T) {
	service := CertificateService{LiveDir: t.TempDir()}
	if err := service.DeployHook(context.Background(), "/tmp/untrusted"); err == nil {
		t.Fatal("DeployHook() error = nil")
	}
}

func TestCertificateService_DeployHookUpdatesKnownLineageAndReloads(t *testing.T) {
	live := filepath.Join(t.TempDir(), "live")
	lineagePath := filepath.Join(live, "external-lineage")
	if err := os.MkdirAll(lineagePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lineagePath, "cert.pem"), []byte("certificate"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, systemd := &certificateStore{}, &certificateSystemd{}
	service := CertificateService{Store: store, FS: system.OSFS{}, Commands: &fake.Commander{Result: system.Result{Stdout: "notAfter=Sep  1 12:00:00 2026 GMT\n"}}, Systemd: systemd, Apache: "apache2", LiveDir: live}
	if err := service.DeployHook(context.Background(), lineagePath); err != nil {
		t.Fatal(err)
	}
	if store.updated != "external-lineage" || systemd.reloaded != "apache2" {
		t.Errorf("store=%#v systemd=%#v", store, systemd)
	}
	systemd.err = errors.New("reload failed")
	if err := service.DeployHook(context.Background(), lineagePath); err == nil {
		t.Fatal("DeployHook() error = nil")
	}
}

func TestSSLService_validateDNS_RequiresEveryDomainToMatchServer(t *testing.T) {
	service := SSLService{Network: certificateNetwork{serverIPs: []string{"192.0.2.10"}, addresses: map[string][]string{"example.test": {"192.0.2.10"}, "www.example.test": {"2001:db8::1"}}}}
	if err := service.validateDNS(context.Background(), []string{"example.test"}); err != nil {
		t.Fatalf("validateDNS() error = %v", err)
	}
	if err := service.validateDNS(context.Background(), []string{"example.test", "www.example.test"}); err == nil {
		t.Fatal("validateDNS() error = nil")
	}
}

func TestSSLService_selfCheck_RequiresApacheNotFound(t *testing.T) {
	service := SSLService{Network: certificateNetwork{status: 404}}
	if err := service.selfCheck(context.Background(), "example.test"); err != nil {
		t.Fatalf("selfCheck() error = %v", err)
	}
	service.Network = certificateNetwork{status: 301}
	if err := service.selfCheck(context.Background(), "example.test"); err == nil {
		t.Fatal("selfCheck() error = nil")
	}
}

func TestSSLService_EnableChecksEveryCertificateNameBeforeIssuance(t *testing.T) {
	requests := []string{}
	service := SSLService{Network: certificateNetwork{status: 404, requests: &requests}}
	for _, name := range []string{"example.test", "www.example.test"} {
		if err := service.selfCheck(context.Background(), name); err != nil {
			t.Fatalf("selfCheck(%q) error = %v", name, err)
		}
	}
	if len(requests) != 2 || !strings.HasPrefix(requests[0], "http://example.test/") || !strings.HasPrefix(requests[1], "http://www.example.test/") {
		t.Errorf("self-check requests = %#v", requests)
	}
}

func TestSSLService_CertbotArgsUsesCompleteSANSet(t *testing.T) {
	service := SSLService{Config: config.Config{Paths: config.Paths{ACMEChallenge: "/acme"}, SSL: config.SSL{Email: "ops@example.test", Staging: true, Server: "https://acme.example.test/directory"}}}
	args := service.certbotArgs("provctl-site-7", []string{"example.test", "www.example.test"}, true)
	joined := strings.Join(args, " ")
	for _, want := range []string{"-d example.test", "-d www.example.test", "--cert-name provctl-site-7", "--expand", "--server https://acme.example.test/directory"} {
		if !strings.Contains(joined, want) {
			t.Errorf("certbot arguments %q do not contain %q", joined, want)
		}
	}
	if strings.Contains(joined, "--staging") {
		t.Errorf("certbot arguments %q contain --staging with an explicit server", joined)
	}
}

func TestSSLService_RenewalCheckArgsUsesPebbleRenewal(t *testing.T) {
	service := SSLService{Config: config.Config{SSL: config.SSL{Server: "https://pebble:14000/dir"}}}
	if got, want := strings.Join(service.renewalCheckArgs("provctl-site-7"), " "), "renew --cert-name provctl-site-7 --force-renewal --no-random-sleep-on-renew"; got != want {
		t.Errorf("renewalCheckArgs() = %q, want %q", got, want)
	}
	service.Config.SSL.Server = ""
	if got, want := strings.Join(service.renewalCheckArgs("provctl-site-7"), " "), "renew --cert-name provctl-site-7 --dry-run"; got != want {
		t.Errorf("renewalCheckArgs() = %q, want %q", got, want)
	}
}
