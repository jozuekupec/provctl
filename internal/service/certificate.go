package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"provctl/internal/audit"
	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

type CertificateStore interface {
	UpdateCertificateNotAfter(context.Context, string, time.Time) (bool, error)
}

type SSLWebsiteStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListWebsites(context.Context, int64) ([]domain.Website, error)
	SetWebsiteSSL(context.Context, int64, bool, bool) error
	CreateCertificate(context.Context, domain.Certificate) (int64, error)
}

// SSLNetwork isolates DNS and HTTP self-check I/O from the certificate state
// machine, keeping its tests unprivileged and deterministic.
type SSLNetwork interface {
	LookupHost(context.Context, string) ([]string, error)
	ServerIPs() ([]string, error)
	Get(context.Context, string) (int, error)
}

type productionSSLNetwork struct{}

func (productionSSLNetwork) LookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func (productionSSLNetwork) ServerIPs() ([]string, error) {
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	addresses := make([]string, 0, len(interfaces))
	for _, address := range interfaces {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && !ip.IsLoopback() {
			addresses = append(addresses, ip.String())
		}
	}
	return addresses, nil
}

func (productionSSLNetwork) Get(ctx context.Context, target string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
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
	Audit    audit.Writer
}

// SSLService coordinates the documented Certbot state machine. Certbot
// issuance is intentionally irreversible; Apache and SQLite changes remain
// rollback-capable around it.
type SSLService struct {
	Store    SSLWebsiteStore
	Network  SSLNetwork
	Commands system.Commander
	FS       system.FS
	Apache   ApacheVHostApplier
	Websites WebsiteService
	Config   config.Config
	Audit    audit.Writer
}

// Enable issues a certificate after making the HTTP ACME endpoint reachable,
// then switches the generated vhost to TLS. DNS mismatch is deliberately a
// guarded warning: NAT deployments may need --force.
func (service SSLService) Enable(ctx context.Context, subscriptionName, primaryDomain string, force, forceHTTPS, renewalCheck bool) (returnErr error) {
	started := time.Now()
	defer func() {
		writeDirectAudit(service.Audit, "ssl.enable", subscriptionName+"/"+primaryDomain, started, returnErr)
	}()
	if service.Store == nil || service.Network == nil || service.Commands == nil || service.FS == nil || service.Apache == nil {
		return errors.New("SSL enable requires store, network, filesystem, commander, and Apache")
	}
	if service.Config.SSL.Email == "" {
		return errors.New("[ssl] email must be set before issuing certificates")
	}
	subscription, website, err := service.website(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return err
	}
	if !website.Enabled {
		return fmt.Errorf("website %q is disabled", primaryDomain)
	}
	if err := service.validateDNS(ctx, append([]string{website.PrimaryDomain}, website.Aliases...)); err != nil && !force {
		return fmt.Errorf("DNS validation warning: %w; rerun with --force when this server is behind NAT", err)
	}
	plain := website
	plain.SSLEnabled = false
	plain.ForceHTTPS = false
	contents, err := service.Websites.RenderVHost(subscriptionName, plain)
	if err != nil {
		return err
	}
	path := service.vhostPath(subscriptionName, primaryDomain)
	undoHTTP, err := service.Apache.Apply(ctx, path, contents)
	if err != nil {
		return err
	}
	if err := service.selfCheck(ctx, primaryDomain); err != nil {
		_ = undoHTTP(ctx)
		return err
	}
	lineage := meta.FilePrefix + subscriptionName + "-" + primaryDomain
	args := []string{"certonly", "--webroot", "-w", service.Config.Paths.ACMEChallenge, "-d", primaryDomain}
	for _, alias := range website.Aliases {
		args = append(args, "-d", alias)
	}
	args = append(args, "--non-interactive", "--agree-tos", "-m", service.Config.SSL.Email, "--cert-name", lineage)
	if service.Config.SSL.Staging {
		args = append(args, "--staging")
	}
	if service.Config.SSL.Server != "" {
		args = append(args, "--server", service.Config.SSL.Server)
	}
	result, err := service.Commands.Run(ctx, "/usr/bin/certbot", args...)
	if err != nil {
		_ = undoHTTP(ctx)
		return commandError("issue certificate", result, err)
	}
	certificatePath := filepath.Join(meta.LetsEncryptLiveDir, lineage, "fullchain.pem")
	if _, err := service.FS.Stat(certificatePath); err != nil {
		return fmt.Errorf("issued certificate %q: %w", certificatePath, err)
	}
	notAfter, err := (CertificateService{FS: service.FS, Commands: service.Commands}).readNotAfter(ctx, certificatePath)
	if err != nil {
		return err
	}
	tls := website
	tls.SSLEnabled, tls.ForceHTTPS = true, forceHTTPS
	contents, err = service.Websites.RenderVHost(subscriptionName, tls)
	if err != nil {
		return err
	}
	if _, err := service.Apache.Apply(ctx, path, contents); err != nil {
		return err
	}
	if err := service.Store.SetWebsiteSSL(ctx, website.ID, true, forceHTTPS); err != nil {
		return err
	}
	if _, err := service.Store.CreateCertificate(ctx, domain.Certificate{SubscriptionID: subscription.ID, Lineage: lineage, PrimaryDomain: primaryDomain, SANs: append([]string{primaryDomain}, website.Aliases...), NotAfter: notAfter, LastCheckedAt: time.Now().UTC()}); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		return err
	}
	if renewalCheck {
		result, err := service.Commands.Run(ctx, "/usr/bin/certbot", "renew", "--cert-name", lineage, "--dry-run")
		if err != nil {
			return fmt.Errorf("certificate issued, but renewal check warning: %w", commandError("verify certificate renewal", result, err))
		}
	}
	return nil
}

// Disable removes TLS from the generated configuration and leaves Certbot's
// lineage untouched to avoid consuming issuance rate limits on re-enable.
func (service SSLService) Disable(ctx context.Context, subscriptionName, primaryDomain string) (returnErr error) {
	started := time.Now()
	defer func() {
		writeDirectAudit(service.Audit, "ssl.disable", subscriptionName+"/"+primaryDomain, started, returnErr)
	}()
	if service.Store == nil || service.Apache == nil {
		return errors.New("SSL disable requires store and Apache")
	}
	_, website, err := service.website(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return err
	}
	plain := website
	plain.SSLEnabled, plain.ForceHTTPS = false, false
	contents, err := service.Websites.RenderVHost(subscriptionName, plain)
	if err != nil {
		return err
	}
	if _, err := service.Apache.Apply(ctx, service.vhostPath(subscriptionName, primaryDomain), contents); err != nil {
		return err
	}
	return service.Store.SetWebsiteSSL(ctx, website.ID, false, false)
}

func (service SSLService) website(ctx context.Context, subscriptionName, primaryDomain string) (domain.Subscription, domain.Website, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return domain.Subscription{}, domain.Website{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return domain.Subscription{}, domain.Website{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Subscription{}, domain.Website{}, err
	}
	websites, err := service.Store.ListWebsites(ctx, subscription.ID)
	if err != nil {
		return domain.Subscription{}, domain.Website{}, err
	}
	for _, website := range websites {
		if website.PrimaryDomain == primaryDomain {
			return subscription, website, nil
		}
	}
	return domain.Subscription{}, domain.Website{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
}

func (service SSLService) validateDNS(ctx context.Context, names []string) error {
	serverIPs, err := service.Network.ServerIPs()
	if err != nil {
		return fmt.Errorf("discover server IPs: %w", err)
	}
	known := make(map[string]bool, len(serverIPs))
	for _, ip := range serverIPs {
		known[ip] = true
	}
	for _, name := range names {
		addresses, err := service.Network.LookupHost(ctx, name)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", name, err)
		}
		matched := false
		for _, address := range addresses {
			if known[address] {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%q does not resolve to a server IP", name)
		}
	}
	return nil
}

func (service SSLService) selfCheck(ctx context.Context, primaryDomain string) error {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return fmt.Errorf("generate ACME self-check token: %w", err)
	}
	status, err := service.Network.Get(ctx, "http://"+primaryDomain+"/.well-known/acme-challenge/"+fmt.Sprintf("%x", random))
	if err != nil {
		return fmt.Errorf("ACME HTTP self-check: %w", err)
	}
	if status != http.StatusNotFound {
		return fmt.Errorf("ACME HTTP self-check returned %d, want 404", status)
	}
	return nil
}

func (service SSLService) vhostPath(subscription, domain string) string {
	return filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription+"-"+domain+".conf")
}

// CertificateRuntime owns the writable SQLite connection required by the
// deploy hook. Status intentionally does not depend on the database.
type CertificateRuntime struct {
	Service    CertificateService
	repository *sqlite.Repository
}

// SSLRuntime owns the writable state used by certificate enable and disable.
type SSLRuntime struct {
	Service    SSLService
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
	return &CertificateRuntime{Service: CertificateService{Store: repository, FS: system.OSFS{}, Commands: commander, Systemd: system.CommandSystemd{Commander: commander}, Apache: apacheService, Audit: audit.FileWriter{Path: meta.AuditLog}}, repository: repository}, nil
}

func NewProductionSSLRuntime(ctx context.Context, cfg config.Config) (*SSLRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	apache := Apache{FS: system.OSFS{}, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service}
	return &SSLRuntime{Service: SSLService{Store: repository, Network: productionSSLNetwork{}, Commands: commander, FS: system.OSFS{}, Apache: apache, Websites: WebsiteService{Config: cfg}, Config: cfg, Audit: audit.FileWriter{Path: meta.AuditLog}}, repository: repository}, nil
}

func (runtime *CertificateRuntime) Close() error { return runtime.repository.Close() }
func (runtime *SSLRuntime) Close() error         { return runtime.repository.Close() }

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
func (service CertificateService) DeployHook(ctx context.Context, lineagePath string) (returnErr error) {
	started := time.Now()
	defer func() {
		writeDirectAudit(service.Audit, "ssl.deploy-hook", filepath.Base(lineagePath), started, returnErr)
	}()
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
	return meta.LetsEncryptLiveDir
}

func commandError(action string, result system.Result, err error) error {
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if output == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, output)
}
