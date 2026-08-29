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
}

type ApacheVHostApplier interface {
	ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error)
}

// WebsiteService creates an isolated PHP-FPM website and its Apache vhost.
type WebsiteService struct {
	FS       system.FS
	Store    WebsiteStore
	Executor plan.Executor
	Apache   ApacheVHostApplier
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
	return &WebsiteRuntime{
		Service: WebsiteService{
			FS:       system.OSFS{},
			Store:    repository,
			Executor: plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}},
			Apache:   Apache{FS: system.OSFS{}, Commands: commander, Systemd: system.CommandSystemd{Commander: commander}, Service: cfg.Apache.Service},
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
	return &WebsiteRuntime{Service: WebsiteService{FS: system.OSFS{}, Store: repository, Config: cfg}, repository: repository}, nil
}

func (runtime *WebsiteRuntime) Close() error { return runtime.repository.Close() }

func (service WebsiteService) CreatePHPFPM(ctx context.Context, subscriptionName, primaryDomain string) (int64, error) {
	operation, err := service.PrepareCreatePHPFPM(ctx, subscriptionName, primaryDomain)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service WebsiteService) PrepareCreatePHPFPM(ctx context.Context, subscriptionName, primaryDomain string) (plan.Plan, error) {
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
	vhostPath := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+primaryDomain+".conf")
	enabledPath := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhostPath))
	contents, err := render.RenderApachePHPFPMHTTP(render.ApacheHTTPVHost{Subscription: subscription.Name, PrimaryDomain: primaryDomain, DocumentRoot: filepath.Join(siteRoot, "public"), AcmeChallengeRoot: service.Config.Paths.ACMEChallenge, FPMSocket: filepath.Join("/run/php", meta.FilePrefix+subscription.Name+".sock"), ProxyTimeout: service.Config.Apache.ProxyTimeout, LogDir: logDir})
	if err != nil {
		return plan.Plan{}, err
	}
	website := domain.Website{SubscriptionID: subscription.ID, Type: domain.WebsitePHPFPM, PrimaryDomain: primaryDomain, DocumentRoot: filepath.Join(siteRoot, "public"), PHPVersion: subscription.PHPVersion, Enabled: true}
	return service.createPHPFPMPlan(subscription, website, siteRoot, logDir, vhostPath, enabledPath, contents), nil
}

func (service WebsiteService) createPHPFPMPlan(subscription domain.Subscription, website domain.Website, siteRoot, logDir, vhostPath, enabledPath string, contents []byte) plan.Plan {
	directories := []struct {
		name, path string
		mode       os.FileMode
	}{
		{"create website root", siteRoot, 0o751}, {"create public directory", filepath.Join(siteRoot, "public"), 0o755}, {"create application directory", filepath.Join(siteRoot, "app"), 0o750}, {"create storage directory", filepath.Join(siteRoot, "storage"), 0o750},
	}
	steps := make([]plan.Step, 0, len(directories)+4)
	for _, directory := range directories {
		directory := directory
		steps = append(steps, plan.Step{Name: directory.name, Preview: fmt.Sprintf("mkdir -m %04o %s; chown %d:%d %s", directory.mode, directory.path, subscription.UnixUID, subscription.UnixUID, directory.path), Do: service.createOwnedDirectory(directory.path, subscription.UnixUID, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "create website log directory", Preview: fmt.Sprintf("mkdir -m 0750 %s; chown root:%d %s", logDir, subscription.UnixUID, logDir), Do: service.createOwnedDirectory(logDir, 0, subscription.UnixUID, 0o750), Undo: func(context.Context) error { return service.FS.Remove(logDir) }})
	for _, name := range []string{"access.log", "error.log"} {
		path := filepath.Join(logDir, name)
		steps = append(steps, plan.Step{Name: "create " + name, Preview: fmt.Sprintf("create 0640 root:%d %s", subscription.UnixUID, path), Do: service.createLogFile(path, subscription.UnixUID), Undo: func(context.Context) error { return service.FS.Remove(path) }})
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
