package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// HealthStore is the read-only state needed to inspect subscriptions and websites.
type HealthStore interface {
	ListSubscriptions(context.Context) ([]domain.Subscription, error)
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListWebsites(context.Context, int64) ([]domain.Website, error)
}

// HealthNetwork keeps DNS and HTTP checks deterministic in unprivileged tests.
type HealthNetwork interface {
	LookupHost(context.Context, string) ([]string, error)
	ServerIPs() ([]string, error)
	Get(context.Context, string, string) (int, error)
}

// HealthService performs read-only operational checks. It never takes the
// operation lock and never attempts to repair a failed check.
type HealthService struct {
	FS       system.FS
	Store    HealthStore
	Commands system.Commander
	Systemd  system.Systemd
	Network  HealthNetwork
	Database interface{ PingContext(context.Context) error }
	Config   config.Config
}

// HealthRuntime owns the read-only database connection used by health.
type HealthRuntime struct {
	Service    HealthService
	repository *sqlite.Repository
}

func NewProductionHealthRuntime(ctx context.Context, cfg config.Config) (*HealthRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	return &HealthRuntime{Service: HealthService{
		FS: system.OSFS{}, Store: repository, Commands: commander,
		Systemd: system.CommandSystemd{Commander: commander}, Network: productionHealthNetwork{},
		Database: repository.DB, Config: cfg,
	}, repository: repository}, nil
}

func (runtime *HealthRuntime) Close() error { return runtime.repository.Close() }

// Run returns global checks and checks for all websites, or the requested scope.
func (service HealthService) Run(ctx context.Context, subscriptionName, primaryDomain string) ([]Check, error) {
	if service.FS == nil || service.Store == nil || service.Commands == nil || service.Systemd == nil || service.Network == nil || service.Database == nil {
		return nil, errors.New("health requires filesystem, store, commander, systemd, network, and database")
	}
	checks := []Check{
		service.checkService(ctx, service.Config.Apache.Service, "Apache service"),
		service.checkApacheConfig(ctx), service.checkDatabase(ctx),
	}
	subscriptions, err := service.subscriptions(ctx, subscriptionName)
	if err != nil {
		return nil, err
	}
	for _, subscription := range subscriptions {
		websites, err := service.Store.ListWebsites(ctx, subscription.ID)
		if err != nil {
			return nil, fmt.Errorf("list websites for subscription %q: %w", subscription.Name, err)
		}
		for _, website := range websites {
			if primaryDomain != "" && website.PrimaryDomain != primaryDomain {
				continue
			}
			checks = append(checks, service.checkWebsite(ctx, subscription, website)...)
		}
	}
	if primaryDomain != "" && len(checks) == 3 {
		return nil, fmt.Errorf("website %q not found", primaryDomain)
	}
	return checks, nil
}

func (service HealthService) subscriptions(ctx context.Context, name string) ([]domain.Subscription, error) {
	if name == "" {
		return service.Store.ListSubscriptions(ctx)
	}
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return nil, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return []domain.Subscription{subscription}, nil
}

func (service HealthService) checkApacheConfig(ctx context.Context) Check {
	result, err := service.Commands.Run(ctx, "/usr/sbin/apachectl", "configtest")
	if err != nil {
		detail := firstLine(result.Stderr)
		if detail == "" {
			detail = err.Error()
		}
		return Check{Name: "Apache configuration", Status: CheckFail, Detail: "configtest failed: " + detail, Hint: "fix Apache configuration before making further changes"}
	}
	return Check{Name: "Apache configuration", Status: CheckOK, Detail: "configtest passed"}
}

func (service HealthService) checkService(ctx context.Context, unit, name string) Check {
	active, err := service.Systemd.IsActive(ctx, unit)
	if err != nil {
		return Check{Name: name, Status: CheckFail, Detail: fmt.Sprintf("cannot inspect %s: %v", unit, err), Hint: "install and start the service"}
	}
	if !active {
		return Check{Name: name, Status: CheckFail, Detail: unit + " is inactive", Hint: "start " + unit}
	}
	return Check{Name: name, Status: CheckOK, Detail: unit + " is active"}
}

func (service HealthService) checkDatabase(ctx context.Context) Check {
	if err := service.Database.PingContext(ctx); err != nil {
		return Check{Name: "SQLite database", Status: CheckFail, Detail: fmt.Sprintf("cannot connect: %v", err), Hint: "restore the provctl database"}
	}
	return Check{Name: "SQLite database", Status: CheckOK, Detail: "read-only query connection verified"}
}

func (service HealthService) checkWebsite(ctx context.Context, subscription domain.Subscription, website domain.Website) []Check {
	prefix := subscription.Name + "/" + website.PrimaryDomain
	checks := []Check{service.checkVHost(prefix, subscription.Name, website), service.checkDocumentRoot(prefix, website)}
	if !website.Enabled {
		return checks
	}
	if website.Type == domain.WebsitePHPFPM {
		checks = append(checks, service.checkFPM(ctx, prefix, subscription, website))
	}
	checks = append(checks, service.checkDNS(ctx, prefix, website.PrimaryDomain), service.checkHTTP(ctx, prefix, website.PrimaryDomain, false))
	if website.SSLEnabled {
		checks = append(checks, service.checkHTTP(ctx, prefix, website.PrimaryDomain, true))
	}
	return checks
}

func (service HealthService) checkVHost(prefix, subscription string, website domain.Website) Check {
	path := filepath.Join(service.Config.Apache.SitesEnabled, meta.FilePrefix+subscription+"-"+website.PrimaryDomain+".conf")
	_, err := service.FS.Stat(path)
	if website.Enabled && err != nil {
		return Check{Name: prefix + " vhost", Status: CheckFail, Detail: "enabled vhost symlink is missing", Hint: "run provctl reconcile"}
	}
	if !website.Enabled && err == nil {
		return Check{Name: prefix + " vhost", Status: CheckWarn, Detail: "disabled website still has an enabled vhost symlink", Hint: "run provctl reconcile"}
	}
	return Check{Name: prefix + " vhost", Status: CheckOK, Detail: map[bool]string{true: "enabled", false: "disabled"}[website.Enabled]}
}

func (service HealthService) checkDocumentRoot(prefix string, website domain.Website) Check {
	info, err := service.FS.Stat(website.DocumentRoot)
	if err != nil || !info.IsDir() {
		return Check{Name: prefix + " document root", Status: CheckFail, Detail: "document root is unavailable", Hint: "restore the site public directory"}
	}
	if info.Mode().Perm() != 0o755 {
		return Check{Name: prefix + " document root", Status: CheckWarn, Detail: fmt.Sprintf("permissions are %04o, expected 0755", info.Mode().Perm()), Hint: "restore the documented public directory permissions"}
	}
	return Check{Name: prefix + " document root", Status: CheckOK, Detail: website.DocumentRoot}
}

func (service HealthService) checkFPM(ctx context.Context, prefix string, subscription domain.Subscription, website domain.Website) Check {
	version := website.PHPVersion
	if version == "" {
		version = subscription.PHPVersion
	}
	if version == "" {
		return Check{Name: prefix + " PHP-FPM", Status: CheckFail, Detail: "PHP version is not configured", Hint: "set a PHP version for the subscription"}
	}
	active, err := service.Systemd.IsActive(ctx, "php"+version+"-fpm")
	if err != nil || !active {
		return Check{Name: prefix + " PHP-FPM", Status: CheckFail, Detail: "PHP-FPM service is inactive", Hint: "start php" + version + "-fpm"}
	}
	socket := phpSocket(subscription.Name)
	info, err := service.FS.Stat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return Check{Name: prefix + " PHP-FPM", Status: CheckFail, Detail: "pool socket is unavailable", Hint: "restart php" + version + "-fpm"}
	}
	if info.Mode().Perm() != 0o660 {
		return Check{Name: prefix + " PHP-FPM", Status: CheckWarn, Detail: fmt.Sprintf("socket permissions are %04o, expected 0660", info.Mode().Perm()), Hint: "restore the generated PHP-FPM pool"}
	}
	return Check{Name: prefix + " PHP-FPM", Status: CheckOK, Detail: "service and pool socket are active"}
}

func (service HealthService) checkDNS(ctx context.Context, prefix, domainName string) Check {
	resolved, err := service.Network.LookupHost(ctx, domainName)
	if err != nil {
		return Check{Name: prefix + " DNS", Status: CheckFail, Detail: fmt.Sprintf("cannot resolve domain: %v", err), Hint: "create matching A or AAAA records"}
	}
	serverIPs, err := service.Network.ServerIPs()
	if err != nil {
		return Check{Name: prefix + " DNS", Status: CheckWarn, Detail: fmt.Sprintf("cannot inspect server IPs: %v", err)}
	}
	for _, address := range resolved {
		if slices.Contains(serverIPs, address) {
			return Check{Name: prefix + " DNS", Status: CheckOK, Detail: address + " matches this server"}
		}
	}
	return Check{Name: prefix + " DNS", Status: CheckWarn, Detail: "resolved addresses do not match this server", Hint: "verify DNS or use the server's public address"}
}

func (service HealthService) checkHTTP(ctx context.Context, prefix, domainName string, tls bool) Check {
	scheme, name := "http", "HTTP"
	if tls {
		scheme, name = "https", "HTTPS"
	}
	status, err := service.Network.Get(ctx, scheme+"://"+domainName+"/", domainName)
	if err != nil {
		return Check{Name: prefix + " " + name, Status: CheckFail, Detail: fmt.Sprintf("request failed: %v", err), Hint: "check DNS, Apache, and the vhost"}
	}
	if status >= http.StatusBadRequest {
		return Check{Name: prefix + " " + name, Status: CheckFail, Detail: fmt.Sprintf("returned HTTP %d", status), Hint: "inspect the website and Apache logs"}
	}
	return Check{Name: prefix + " " + name, Status: CheckOK, Detail: fmt.Sprintf("returned HTTP %d", status)}
}

type productionHealthNetwork struct{}

func (productionHealthNetwork) LookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func (productionHealthNetwork) ServerIPs() ([]string, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && !ip.IsLoopback() {
			result = append(result, ip.String())
		}
	}
	return result, nil
}

func (productionHealthNetwork) Get(ctx context.Context, target, host string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	request.Host = host
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

var _ interface{ PingContext(context.Context) error } = (*sql.DB)(nil)
