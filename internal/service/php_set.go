package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/render"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

// PHPSettingsStore is the source of truth for a PHP-FPM version change.
type PHPSettingsStore interface {
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	ListWebsites(context.Context, int64) ([]domain.Website, error)
	UpdateWebsitePHPVersion(context.Context, int64, string) error
}

// PHPSetOptions holds the requested per-domain runtime settings.
type PHPSetOptions struct {
	Version string
}

// PHPService changes one website's PHP-FPM pool while preserving a rollback path.
type PHPService struct {
	FS       system.FS
	Systemd  system.Systemd
	Store    PHPSettingsStore
	PHPFPM   PHPFPMPoolApplier
	Apache   ApacheVHostApplier
	Executor plan.Executor
	Config   config.Config
}

// PHPRuntime owns the database connection used by PHP commands.
type PHPRuntime struct {
	Service    PHPService
	repository *sqlite.Repository
}

func NewProductionPHPRuntime(ctx context.Context, cfg config.Config) (*PHPRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return &PHPRuntime{Service: PHPService{
		FS: system.OSFS{}, Systemd: systemd, Store: repository,
		PHPFPM:   PHPFPM{FS: system.OSFS{}, Commands: commander, Systemd: systemd},
		Apache:   Apache{FS: system.OSFS{}, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service},
		Executor: productionExecutor(repository), Config: cfg,
	}, repository: repository}, nil
}

func NewReadOnlyPHPRuntime(ctx context.Context, cfg config.Config) (*PHPRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return &PHPRuntime{Service: PHPService{
		FS: system.OSFS{}, Systemd: systemd, Store: repository,
		PHPFPM: PHPFPM{FS: system.OSFS{}, Commands: commander, Systemd: systemd},
		Apache: Apache{FS: system.OSFS{}, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service},
		Config: cfg,
	}, repository: repository}, nil
}

func (runtime *PHPRuntime) Close() error { return runtime.repository.Close() }

// ListVersions discovers installed FPM versions for a read-only CLI command.
func (service PHPService) ListVersions(ctx context.Context) ([]PHPFPMVersion, error) {
	if service.FS == nil || service.Systemd == nil {
		return nil, fmt.Errorf("filesystem and systemd are required")
	}
	return DiscoverPHPFPM(ctx, service.FS, service.Systemd)
}

func (service PHPService) Set(ctx context.Context, subscriptionName, primaryDomain string, options PHPSetOptions) (int64, error) {
	operation, err := service.PrepareSet(ctx, subscriptionName, primaryDomain, options)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// PrepareSet reads all state and builds a change plan without writing anything.
func (service PHPService) PrepareSet(ctx context.Context, subscriptionName, primaryDomain string, options PHPSetOptions) (plan.Plan, error) {
	if err := domain.ValidateSubscriptionName(subscriptionName); err != nil {
		return plan.Plan{}, err
	}
	if err := domain.ValidateDomain(primaryDomain); err != nil {
		return plan.Plan{}, err
	}
	if !phpVersion.MatchString(options.Version) {
		return plan.Plan{}, fmt.Errorf("PHP-FPM version %q must use major.minor form", options.Version)
	}
	if service.FS == nil || service.Systemd == nil || service.Store == nil {
		return plan.Plan{}, fmt.Errorf("filesystem, systemd, and PHP settings store are required")
	}
	if service.PHPFPM == nil || service.Apache == nil {
		return plan.Plan{}, fmt.Errorf("PHP-FPM pool and Apache vhost appliers are required")
	}
	subscription, err := service.Store.SubscriptionByName(ctx, subscriptionName)
	if err != nil {
		return plan.Plan{}, err
	}
	if subscription.Status != "active" {
		return plan.Plan{}, fmt.Errorf("subscription %q is %s", subscriptionName, subscription.Status)
	}
	versions, err := service.ListVersions(ctx)
	if err != nil {
		return plan.Plan{}, err
	}
	newVersion, err := SelectPHPFPM(options.Version, versions)
	if err != nil {
		return plan.Plan{}, err
	}
	websites, err := service.Store.ListWebsites(ctx, subscription.ID)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("list subscription websites: %w", err)
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
	if website.Type != domain.WebsitePHPFPM {
		return plan.Plan{}, fmt.Errorf("website %q is %s, not PHP-FPM", primaryDomain, website.Type)
	}
	if website.PHPVersion == options.Version {
		return plan.Plan{}, fmt.Errorf("website %q already uses PHP-FPM %s", primaryDomain, options.Version)
	}
	oldVersion, err := SelectPHPFPM(website.PHPVersion, versions)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("find current PHP-FPM version: %w", err)
	}
	desired := website
	desired.PHPVersion = newVersion.Version
	poolContents, err := render.RenderPHPFPMPool(render.PHPFPMPool{Name: phpPoolName(subscription.Name, primaryDomain), User: subscription.UnixUser, Home: subscription.Home, Socket: phpSocket(subscription.Name, primaryDomain), MaxChildren: subscription.PHPMaxChildren, MemoryLimit: subscription.PHPMemoryLimit, UploadMax: subscription.PHPUploadMax, MaxExecTime: subscription.PHPMaxExecTime, PhpErrorLog: phpErrorLog(subscription.Name, primaryDomain)})
	if err != nil {
		return plan.Plan{}, err
	}
	return service.setPlan(subscription, website, desired, oldVersion, newVersion, poolContents)
}

func phpPoolName(subscription, primaryDomain string) string {
	return meta.FilePrefix + subscription + "-" + primaryDomain
}

func phpSocket(subscription, primaryDomain string) string {
	return filepath.Join("/run/php", phpPoolName(subscription, primaryDomain)+".sock")
}

func phpPoolPath(version PHPFPMVersion, subscription, primaryDomain string) string {
	return filepath.Join("/etc/php", version.Version, "fpm", "pool.d", phpPoolName(subscription, primaryDomain)+".conf")
}

func phpErrorLog(subscription, primaryDomain string) string {
	return filepath.Join(phpLogDir(subscription, primaryDomain), "php-fpm-error.log")
}

func phpLogDir(subscription, primaryDomain string) string {
	return filepath.Join(meta.LogDir, subscription, "fpm", primaryDomain)
}

func (service PHPService) setPlan(subscription domain.Subscription, previous, desired domain.Website, oldVersion, newVersion PHPFPMVersion, poolContents []byte) (plan.Plan, error) {
	newPoolPath, oldPoolPath := phpPoolPath(newVersion, subscription.Name, desired.PrimaryDomain), phpPoolPath(oldVersion, subscription.Name, desired.PrimaryDomain)
	socket := phpSocket(subscription.Name, desired.PrimaryDomain)
	steps := make([]plan.Step, 0, 5)
	if oldVersion.Version != newVersion.Version {
		oldPoolExists := service.FS == nil
		if service.FS != nil {
			if _, err := service.FS.Stat(oldPoolPath); err == nil {
				oldPoolExists = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return plan.Plan{}, fmt.Errorf("inspect existing PHP-FPM pool %q: %w", oldPoolPath, err)
			}
		}
		if oldPoolExists {
			var undoOldPool func(context.Context) error
			steps = append(steps, plan.Step{Name: "remove previous PHP-FPM pool", Preview: fmt.Sprintf("remove %s; validate %s -t; reload %s", oldPoolPath, oldVersion.Binary, oldVersion.Service), Do: func(ctx context.Context) error {
				var err error
				undoOldPool, err = service.PHPFPM.RemovePool(ctx, oldVersion, oldPoolPath)
				return err
			}, Undo: func(ctx context.Context) error {
				if undoOldPool == nil {
					return nil
				}
				return undoOldPool(ctx)
			}})
			steps = append(steps, plan.Step{Name: "wait for PHP-FPM socket release", Preview: "wait for " + socket + " to be released by " + oldVersion.Service, Do: func(ctx context.Context) error {
				return waitForSocketRelease(ctx, service.FS, socket)
			}, Undo: func(context.Context) error { return nil }})
		}
	}
	var undoNewPool func(context.Context) error
	steps = append(steps, plan.Step{Name: "install new PHP-FPM pool", Preview: fmt.Sprintf("write %s; validate %s -t; reload %s", newPoolPath, newVersion.Binary, newVersion.Service), Do: func(ctx context.Context) error {
		var err error
		undoNewPool, err = service.PHPFPM.ApplyPool(ctx, newVersion, newPoolPath, poolContents, socket)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoNewPool == nil {
			return nil
		}
		return undoNewPool(ctx)
	}})
	contents, err := (WebsiteService{Config: service.Config}).RenderVHost(subscription.Name, desired)
	if err != nil {
		return plan.Plan{}, err
	}
	path := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+subscription.Name+"-"+desired.PrimaryDomain+".conf")
	var undoApache func(context.Context) error
	steps = append(steps, plan.Step{Name: "regenerate Apache vhost " + desired.PrimaryDomain, Preview: "write " + path, Do: func(ctx context.Context) error {
		var applyErr error
		undoApache, applyErr = service.Apache.Apply(ctx, path, contents)
		return applyErr
	}, Undo: func(ctx context.Context) error {
		if undoApache == nil {
			return nil
		}
		return undoApache(ctx)
	}})
	steps = append(steps, plan.Step{Name: "record PHP-FPM version in SQLite", Preview: "update PHP-FPM website version in SQLite", Do: func(ctx context.Context) error {
		return service.Store.UpdateWebsitePHPVersion(ctx, desired.ID, desired.PHPVersion)
	}, Undo: func(ctx context.Context) error {
		return service.Store.UpdateWebsitePHPVersion(ctx, previous.ID, previous.PHPVersion)
	}})
	return plan.Plan{Action: "php.set", Target: subscription.Name + "/" + desired.PrimaryDomain, Steps: steps}, nil
}

func waitForSocketRelease(ctx context.Context, fs system.FS, socket string) error {
	if fs == nil {
		return nil
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := fs.Stat(socket)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect PHP-FPM socket %q: %w", socket, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("PHP-FPM socket %q was not released within 10 seconds", socket)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
