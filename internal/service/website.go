package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/render"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

type WebsiteStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	DomainExists(context.Context, string) (bool, error)
	CreateWebsite(context.Context, domain.Website) (int64, error)
	DeleteWebsite(context.Context, int64) error
	SetWebsiteEnabled(context.Context, int64, bool) error
	ListWebsites(context.Context, int64) ([]domain.Website, error)
}

// List returns websites of a subscription for read-only callers such as the TUI.
func (service WebsiteService) List(ctx context.Context, subscriptionID int64) ([]domain.Website, error) {
	websites, err := service.Store.ListWebsites(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("list websites: %w", err)
	}
	return websites, nil
}

// ListForSubscription returns websites identified by their subscription name.
func (service WebsiteService) ListForSubscription(ctx context.Context, subscriptionName string) ([]domain.Website, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return nil, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return nil, err
	}
	return service.List(ctx, subscription.ID)
}

type ApacheVHostApplier interface {
	ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error)
	SetVHostEnabled(context.Context, string, string, bool) (func(context.Context) error, error)
	RemoveVHost(context.Context, string, string) (func(context.Context) error, error)
}

// WebsiteService creates an isolated PHP-FPM website and its Apache vhost.
type WebsiteService struct {
	FS       system.FS
	Store    WebsiteStore
	Executor plan.Executor
	Apache   ApacheVHostApplier
	PHPFPM   PHPFPMPoolApplier
	Version  PHPFPMVersion
	Config   config.Config
}

// WebsiteRuntime owns the database connection used by a website command.
type WebsiteRuntime struct {
	Service    WebsiteService
	repository *sqlite.Repository
}

func NewProductionWebsiteRuntime(ctx context.Context, cfg config.Config) (*WebsiteRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	version, err := selectPHPFPM(ctx, cfg, system.OSFS{}, systemd)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &WebsiteRuntime{
		Service: WebsiteService{
			FS:       system.OSFS{},
			Store:    repository,
			Executor: plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}},
			Apache:   Apache{FS: system.OSFS{}, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service},
			PHPFPM:   PHPFPM{FS: system.OSFS{}, Commands: commander, Systemd: systemd},
			Version:  version,
			Config:   cfg,
		},
		repository: repository,
	}, nil
}

func NewReadOnlyWebsiteRuntime(ctx context.Context, cfg config.Config) (*WebsiteRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	version, err := selectPHPFPM(ctx, cfg, system.OSFS{}, systemd)
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &WebsiteRuntime{Service: WebsiteService{FS: system.OSFS{}, Store: repository, PHPFPM: PHPFPM{FS: system.OSFS{}, Commands: commander, Systemd: systemd}, Version: version, Config: cfg}, repository: repository}, nil
}

func (runtime *WebsiteRuntime) Close() error { return runtime.repository.Close() }

func selectPHPFPM(ctx context.Context, cfg config.Config, fs system.FS, systemd system.Systemd) (PHPFPMVersion, error) {
	versions, err := DiscoverPHPFPM(ctx, fs, systemd)
	if err != nil {
		return PHPFPMVersion{}, err
	}
	return SelectPHPFPM(cfg.PHP.DefaultVersion, versions)
}

func (service WebsiteService) CreatePHPFPM(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareCreatePHPFPM(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// CreateStatic provisions an isolated static website and its Apache vhost.
func (service WebsiteService) CreateStatic(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareCreateStatic(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// SetEnabled changes one website's Apache vhost and persisted enabled state.
func (service WebsiteService) SetEnabled(ctx context.Context, subscriptionName, primaryDomain string, enabled bool) (int64, error) {
	operation, err := service.PrepareSetEnabled(ctx, subscriptionName, primaryDomain, enabled)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// Delete removes generated Apache artifacts and the website record. Site data
// and logs are intentionally retained for administrator-managed recovery.
func (service WebsiteService) Delete(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareDelete(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareDelete(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	websites, err := service.ListForSubscription(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	var website domain.Website
	for _, candidate := range websites {
		if candidate.PrimaryDomain == primaryDomain {
			website = candidate
			break
		}
	}
	if website.ID == 0 {
		return plan.Plan{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: "remove Apache vhost", Preview: "remove " + vhostPath, Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.RemoveVHost(ctx, vhostPath, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error { return undoApache(ctx) }}, {Name: "remove website from SQLite", Preview: "delete website and domains from SQLite; retain site data and logs", Do: func(ctx context.Context) error {
		return service.Store.DeleteWebsite(ctx, website.ID)
	}, Undo: func(ctx context.Context) error {
		_, err := service.Store.CreateWebsite(ctx, website)
		return err
	}}}
	return plan.Plan{Action: "website.delete", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

// PrepareSetEnabled builds a reversible enable or disable operation.
func (service WebsiteService) PrepareSetEnabled(ctx context.Context, subscriptionName, primaryDomain string, enabled bool) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	websites, err := service.ListForSubscription(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	var website domain.Website
	found := false
	for _, candidate := range websites {
		if candidate.PrimaryDomain == primaryDomain {
			website, found = candidate, true
			break
		}
	}
	if !found {
		return plan.Plan{}, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscriptionName)
	}
	if website.Enabled == enabled {
		return plan.Plan{}, fmt.Errorf("website %q is already enabled=%t", primaryDomain, enabled)
	}
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscriptionName+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	var undoApache func(context.Context) error
	steps := []plan.Step{{Name: map[bool]string{true: "enable Apache vhost", false: "disable Apache vhost"}[enabled], Preview: fmt.Sprintf("set %s enabled=%t", enabledPath, enabled), Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.SetVHostEnabled(ctx, vhostPath, enabledPath, enabled)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, {Name: "record website enabled state", Preview: fmt.Sprintf("set website %d enabled=%t in SQLite", website.ID, enabled), Do: func(ctx context.Context) error {
		return service.Store.SetWebsiteEnabled(ctx, website.ID, enabled)
	}, Undo: func(ctx context.Context) error {
		return service.Store.SetWebsiteEnabled(ctx, website.ID, website.Enabled)
	}}}
	return plan.Plan{Action: "website.set-enabled", Target: subscriptionName + "/" + primaryDomain, Steps: steps}, nil
}

func (service WebsiteService) CreateProxy(ctx context.Context, subscriptionName, primaryDomain, target string) (int64, error) {
	operation, err := service.PrepareCreateProxy(ctx, subscriptionName, primaryDomain, target)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareCreateProxy(ctx context.Context, subscriptionName, primaryDomain, target string) (plan.Plan, error) {
	subscription, logDir, vhostPath, enabledPath, err := service.prepareHTTPWebsite(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	contents, err := render.RenderApacheProxyHTTP(render.ApacheProxyVHost{PrimaryDomain: primaryDomain, Target: target, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir}, service.Config.Apache.AllowedProxyHosts)
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteProxy, PrimaryDomain: primaryDomain, Target: target, Enabled: true}
	return service.createHTTPOnlyPlan(subscription, website, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) CreateRedirect(ctx context.Context, subscriptionName, primaryDomain, target string, code int) (int64, error) {
	operation, err := service.PrepareCreateRedirect(ctx, subscriptionName, primaryDomain, target, code)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareCreateRedirect(ctx context.Context, subscriptionName, primaryDomain, target string, code int) (plan.Plan, error) {
	subscription, logDir, vhostPath, enabledPath, err := service.prepareHTTPWebsite(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return plan.Plan{}, err
	}
	contents, err := render.RenderApacheRedirectHTTP(render.ApacheRedirectVHost{PrimaryDomain: primaryDomain, Target: target, RedirectCode: code, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteRedirect, PrimaryDomain: primaryDomain, Target: target, RedirectCode: code, Enabled: true}
	return service.createHTTPOnlyPlan(subscription, website, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) prepareHTTPWebsite(ctx context.Context, subscriptionName, primaryDomain string) (domain.Subscription, string, string, string, error) {
	if service.Apache == nil {
		return domain.Subscription{}, "", "", "", fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return domain.Subscription{}, "", "", "", err
	}
	if subscription.Status != "active" {
		return domain.Subscription{}, "", "", "", fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	exists, err := service.Store.DomainExists(ctx, primaryDomain)
	if err != nil {
		return domain.Subscription{}, "", "", "", fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return domain.Subscription{}, "", "", "", fmt.Errorf("domain %q is already assigned", primaryDomain)
	}
	logDir := filepath.Join(meta.LogDir, subscription.Name, primaryDomain)
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	return subscription, logDir, vhostPath, filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath)), nil
}

func (service WebsiteService) createHTTPOnlyPlan(subscription domain.Subscription, website domain.Website, logDir, vhostPath, enabledPath string, contents []byte) plan.Plan {
	steps := []plan.Step{{Name: "create website log directory", Preview: "create " + logDir, Do: service.createOwnedDirectory(logDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(logDir) }}}
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: "create " + path, Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: "write " + vhostPath, Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, contents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}}, plan.Step{Name: "record website", Preview: "insert website and primary domain into SQLite", Do: func(ctx context.Context) error {
		id, err := service.Store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteWebsite(ctx, website.ID) }})
	return plan.Plan{Action: "website.create", Target: subscription.Name + "/" + website.PrimaryDomain, Steps: steps}
}

// PrepareCreateStatic validates state and prepares a static website without changes.
func (service WebsiteService) PrepareCreateStatic(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
	if service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("Apache vhost applier is required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	if subscription.Status != "active" {
		return plan.Plan{}, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	exists, err := service.Store.DomainExists(ctx, primaryDomain)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return plan.Plan{}, fmt.Errorf("domain %q is already assigned", primaryDomain)
	}
	siteRoot := filepath.Join(subscription.Home, "sites", primaryDomain)
	documentRoot := filepath.Join(siteRoot, "public")
	logDir := filepath.Join(meta.LogDir, subscription.Name, primaryDomain)
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	contents, err := render.RenderApacheStaticHTTP(render.ApacheStaticVHost{PrimaryDomain: primaryDomain, DocumentRoot: documentRoot, AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsiteStatic, PrimaryDomain: primaryDomain, DocumentRoot: documentRoot, Enabled: true}
	return service.createStaticPlan(subscription, website, siteRoot, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) PrepareCreatePHPFPM(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
	if service.PHPFPM == nil || service.Version.Version == "" || service.Version.Binary == "" || service.Version.Service == "" {
		return plan.Plan{}, fmt.Errorf("PHP-FPM version and pool applier are required")
	}
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	if subscription.Status != "active" {
		return plan.Plan{}, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	exists, err := service.Store.DomainExists(ctx, primaryDomain)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("check domain: %w", err)
	}
	if exists {
		return plan.Plan{}, fmt.Errorf("domain %q is already assigned", primaryDomain)
	}
	siteRoot := filepath.Join(subscription.Home, "sites", primaryDomain)
	logDir := filepath.Join(meta.LogDir, subscription.Name, primaryDomain)
	fpmLogDir := filepath.Join(meta.LogDir, subscription.Name)
	fpmErrorLog := filepath.Join(fpmLogDir, "php-fpm-error.log")
	socket := filepath.Join("/run/php", meta.FilePrefix+subscription.Name+".sock")
	poolPath := filepath.Join("/etc/php", service.Version.Version, "fpm", "pool.d", meta.FilePrefix+subscription.Name+".conf")
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	contents, err := render.RenderApachePHPFPMHTTP(render.ApacheHTTPVHost{Subscription: subscription.Name, PrimaryDomain: primaryDomain, DocumentRoot: filepath.Join(siteRoot, "public"), AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, FPMSocket: socket, ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	poolContents, err := render.RenderPHPFPMPool(render.PHPFPMPool{Name: subscription.Name, Home: subscription.Home, Socket: socket, MaxChildren: subscription.PHPMaxChildren, MemoryLimit: subscription.PHPMemoryLimit, UploadMax: subscription.PHPUploadMax, MaxExecTime: subscription.PHPMaxExecTime, PhpErrorLog: fpmErrorLog})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsitePHPFPM, PrimaryDomain: primaryDomain, DocumentRoot: filepath.Join(siteRoot, "public"), PHPVersion: service.Version.Version, Enabled: true}
	return service.createPHPFPMPlan(subscription, website, siteRoot, logDir, fpmLogDir, fpmErrorLog, poolPath, socket, poolContents, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) createPHPFPMPlan(subscription domain.Subscription, website domain.Website, siteRoot, logDir, fpmLogDir, fpmErrorLog, poolPath, socket string, poolContents []byte, vhostPath, enabledPath string, contents []byte) plan.Plan {
	directories := []struct {
		name, path string
		mode       os.FileMode
	}{
		{"create website root", siteRoot, 0o751}, {"create public directory", filepath.Join(siteRoot, "public"), 0o755}, {"create application directory", filepath.Join(siteRoot, "app"), 0o750}, {"create storage directory", filepath.Join(siteRoot, "storage"), 0o750},
	}
	steps := make([]plan.Step, 0, len(directories)+7)
	for _, directory := range directories {
		directory := directory
		steps = append(steps, plan.Step{Name: directory.name, Preview: fmt.Sprintf("mkdir -m %04o %s; chown %d:%d %s", directory.mode, directory.path, subscription.UnixUID, subscription.UnixUID, directory.path), Do: service.createOwnedDirectory(directory.path, subscription.UnixUID, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "create PHP-FPM log directory", Preview: fmt.Sprintf("mkdir -m 0750 %s; chown root:%d %s", fpmLogDir, subscription.UnixUID, fpmLogDir), Do: service.createOwnedDirectory(fpmLogDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(fpmLogDir) }})
	steps = append(steps, plan.Step{Name: "create PHP-FPM error log", Preview: fmt.Sprintf("create 0600 %d:%d %s", subscription.UnixUID, subscription.UnixUID, fpmErrorLog), Do: service.createPHPErrorLog(fpmErrorLog, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(fpmErrorLog) }})
	steps = append(steps, plan.Step{Name: "create website log directory", Preview: fmt.Sprintf("mkdir -m 0750 %s; chown root:%d %s", logDir, subscription.UnixUID, logDir), Do: service.createOwnedDirectory(logDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(logDir) }})
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: fmt.Sprintf("create 0640 root:%d %s", subscription.UnixUID, path), Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoPool func(context.Context) error
	steps = append(steps, plan.Step{Name: "install PHP-FPM pool", Preview: fmt.Sprintf("write %s; validate %s -t; reload %s", poolPath, service.Version.Binary, service.Version.Service), Do: func(ctx context.Context) error {
		var err error
		undoPool, err = service.PHPFPM.ApplyPool(ctx, service.Version, poolPath, poolContents, socket)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoPool == nil {
			return nil
		}
		return undoPool(ctx)
	}})
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: fmt.Sprintf("write %s and enable %s", vhostPath, enabledPath), Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, contents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}})
	steps = append(steps, plan.Step{Name: "record website", Preview: "insert website and primary domain into SQLite", Do: func(ctx context.Context) error {
		id, err := service.Store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteWebsite(ctx, website.ID) }})
	return plan.Plan{Action: "website.create", Target: subscription.Name + "/" + website.PrimaryDomain, Steps: steps}
}

func (service WebsiteService) createStaticPlan(subscription domain.Subscription, website domain.Website, siteRoot, logDir, vhostPath, enabledPath string, contents []byte) plan.Plan {
	directories := []struct {
		name, path string
		mode       os.FileMode
	}{{"create website root", siteRoot, 0o751}, {"create public directory", filepath.Join(siteRoot, "public"), 0o755}, {"create website log directory", logDir, 0o750}}
	steps := make([]plan.Step, 0, len(directories)+4)
	for _, directory := range directories {
		directory := directory
		uid, gid := subscription.UnixUID, subscription.UnixUID
		if directory.path == logDir {
			uid = 0
		}
		steps = append(steps, plan.Step{Name: directory.name, Preview: fmt.Sprintf("mkdir -m %04o %s", directory.mode, directory.path), Do: service.createOwnedDirectory(directory.path, uid, gid, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: "create " + path, Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
	}
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "install and enable Apache vhost", Preview: fmt.Sprintf("write %s and enable %s", vhostPath, enabledPath), Do: func(ctx context.Context) error {
		var err error
		undoApache, err = service.Apache.ApplyVHost(ctx, vhostPath, contents, enabledPath)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}})
	steps = append(steps, plan.Step{Name: "record website", Preview: "insert website and primary domain into SQLite", Do: func(ctx context.Context) error {
		id, err := service.Store.CreateWebsite(ctx, website)
		website.ID = id
		return err
	}, Undo: func(ctx context.Context) error { return service.Store.DeleteWebsite(ctx, website.ID) }})
	return plan.Plan{Action: "website.create", Target: subscription.Name + "/" + website.PrimaryDomain, Steps: steps}
}

func (service WebsiteService) createOwnedDirectory(path string, uid, gid int, mode os.FileMode) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.MkdirAll(path, mode); err != nil {
			return err
		}
		if err := service.FS.Chown(path, uid, gid); err != nil {
			return err
		}
		return service.FS.Chmod(path, mode)
	}
}

func (service WebsiteService) createLogFile(path string, gid int) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.WriteFileAtomic(path, nil, 0o640); err != nil {
			return err
		}
		if err := service.FS.Chown(path, 0, gid); err != nil {
			return err
		}
		return service.FS.Chmod(path, 0o640)
	}
}

func (service WebsiteService) createPHPErrorLog(path string, uid int) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.WriteFileAtomic(path, nil, 0o600); err != nil {
			return err
		}
		if err := service.FS.Chown(path, uid, uid); err != nil {
			return err
		}
		return service.FS.Chmod(path, 0o600)
	}
}
