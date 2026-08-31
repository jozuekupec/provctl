package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

const letsEncryptLiveDir = "/etc/letsencrypt/live"

type CertificateStore interface {
	UpdateCertificateNotAfter(context.Context, string, time.Time) (bool, error)
}

// SSLStatus reports the certificate state read directly from Certbot's live
// lineage. SQLite is deliberately not consulted for this read-only status.
type SSLStatus struct {
	Lineage  string
	NotAfter time.Time
}

// CertificateService owns small, idempotent certificate maintenance actions.
type CertificateService struct {
	Store    CertificateStore
	FS       system.FS
	Commands system.Commander
	Systemd  system.Systemd
	Apache   string
	LiveDir  string
}

// CertificateRuntime owns the writable SQLite connection required by the
// deploy hook. Status intentionally does not depend on the database.
type CertificateRuntime struct {
	Service    CertificateService
	repository *sqlite.Repository
}

// NewCertificateStatusService constructs the read-only service used by the
// CLI. It deliberately has no SQLite dependency because live files are the
// authority for certificate status.
func NewCertificateStatusService() CertificateService {
	return CertificateService{FS: system.OSFS{}, Commands: system.ExecCommander{}}
}

func NewProductionCertificateRuntime(ctx context.Context, apacheService string) (*CertificateRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	return &CertificateRuntime{Service: CertificateService{Store: repository, FS: system.OSFS{}, Commands: commander, Systemd: system.CommandSystemd{Commander: commander}, Apache: apacheService}, repository: repository}, nil
}

func (runtime *CertificateRuntime) Close() error { return runtime.repository.Close() }

func (service CertificateService) Status(ctx context.Context, subscription, primaryDomain string) (SSLStatus, error) {
	if err := domain.ValidateSubscriptionName(subscription); err != nil {
		return SSLStatus{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return SSLStatus{}, err
	}
	lineage := meta.FilePrefix + subscription + "-" + primaryDomain
	notAfter, err := service.readNotAfter(ctx, filepath.Join(service.liveDir(), lineage, "cert.pem"))
	if err != nil {
		return SSLStatus{}, err
	}
	return SSLStatus{Lineage: lineage, NotAfter: notAfter}, nil
}

// DeployHook updates the cached expiry when the lineage is known and reloads
// Apache. It intentionally succeeds for a Certbot lineage not managed by
// provctl so the global hook also covers externally issued certificates.
func (service CertificateService) DeployHook(ctx context.Context, lineagePath string) error {
	if service.Store == nil || service.Systemd == nil || service.Commands == nil {
		return errors.New("certificate hook requires store, commander, and systemd")
	}
	lineage, err := service.lineageFromPath(lineagePath)
	if err != nil {
		return err
	}
	notAfter, err := service.readNotAfter(ctx, filepath.Join(service.liveDir(), lineage, "cert.pem"))
	if err != nil {
		return err
	}
	if _, err := service.Store.UpdateCertificateNotAfter(ctx, lineage, notAfter); err != nil {
		return err
	}
	if service.Apache == "" {
		return errors.New("Apache service is required")
	}
	if err := service.Systemd.Reload(ctx, service.Apache); err != nil {
		return fmt.Errorf("reload Apache after certificate renewal: %w", err)
	}
	return nil
}

func (service CertificateService) readNotAfter(ctx context.Context, certificatePath string) (time.Time, error) {
	if service.FS == nil || service.Commands == nil {
		return time.Time{}, errors.New("certificate status requires filesystem and commander")
	}
	if _, err := service.FS.Stat(certificatePath); err != nil {
		return time.Time{}, fmt.Errorf("certificate %q: %w", certificatePath, err)
	}
	result, err := service.Commands.Run(ctx, "/usr/bin/openssl", "x509", "-in", certificatePath, "-noout", "-enddate")
	if err != nil {
		return time.Time{}, commandError("read certificate expiry", result, err)
	}
	value := strings.TrimSpace(result.Stdout)
	value = strings.TrimPrefix(value, "notAfter=")
	notAfter, err := time.Parse("Jan 2 15:04:05 2006 MST", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse certificate expiry %q: %w", value, err)
	}
	return notAfter.UTC(), nil
}

func (service CertificateService) lineageFromPath(lineagePath string) (string, error) {
	clean := filepath.Clean(lineagePath)
	if filepath.Dir(clean) != service.liveDir() {
		return "", fmt.Errorf("certificate lineage must be directly below %q", service.liveDir())
	}
	lineage := filepath.Base(clean)
	if lineage == "." || lineage == string(filepath.Separator) || strings.Contains(lineage, string(filepath.Separator)) {
		return "", fmt.Errorf("invalid certificate lineage path")
	}
	return lineage, nil
}

func (service CertificateService) liveDir() string {
	if service.LiveDir != "" {
		return service.LiveDir
	}
	return letsEncryptLiveDir
}

func commandError(action string, result system.Result, err error) error {
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if output == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, output)
}
