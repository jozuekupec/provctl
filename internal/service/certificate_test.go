package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"provctl/internal/system"
	"provctl/internal/system/fake"
)

type certificateStore struct {
	updated  string
	notAfter time.Time
}

func (store *certificateStore) UpdateCertificateNotAfter(_ context.Context, lineage string, notAfter time.Time) (bool, error) {
	store.updated, store.notAfter = lineage, notAfter
	return true, nil
}

type certificateSystemd struct {
	reloaded string
	err      error
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
