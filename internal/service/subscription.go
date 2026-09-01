package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

type SubscriptionStore interface {
	SubscriptionExists(context.Context, string) (bool, error)
	SubscriptionUIDExists(context.Context, int) (bool, error)
	ListSubscriptions(context.Context) ([]domain.Subscription, error)
	SubscriptionByName(context.Context, string) (domain.Subscription, error)
	CreateSubscription(context.Context, domain.Subscription) error
	DeleteSubscription(context.Context, string) error
	SetSubscriptionStatus(context.Context, int64, string) error
}

func (service SubscriptionService) List(ctx context.Context) ([]domain.Subscription, error) {
	subscriptions, err := service.Store.ListSubscriptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subscriptions, nil
}

func (service SubscriptionService) Show(ctx context.Context, name string) (domain.Subscription, error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return domain.Subscription{}, err
	}
	subscription, err := service.Store.SubscriptionByName(ctx, name)
	if err != nil {
		return domain.Subscription{}, err
	}
	return subscription, nil
}

type SubscriptionService struct {
	FS         system.FS
	Users      system.Users
	Store      SubscriptionStore
	Executor   plan.Executor
	PHPVersion string
	Config     config.Config
}

type SubscriptionCreateOptions struct {
	QuotaDiskBytes int64
	QuotaWebsites  int
	QuotaDatabases int
	QuotaBackups   int
}

// SubscriptionRuntime owns the database connection used by a production command.
type SubscriptionRuntime struct {
	Service    SubscriptionService
	repository *sqlite.Repository
}

func NewProductionSubscriptionRuntime(ctx context.Context, cfg config.Config) (*SubscriptionRuntime, error) {
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	version, err := selectPHPFPM(ctx, cfg, system.OSFS{}, system.CommandSystemd{Commander: commander})
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &SubscriptionRuntime{
		Service: SubscriptionService{
			FS:         system.OSFS{},
			Users:      system.CommandUsers{Commander: commander},
			Store:      repository,
			Executor:   productionExecutor(repository),
			PHPVersion: version.Version,
			Config:     cfg,
		},
		repository: repository,
	}, nil
}

func NewReadOnlySubscriptionRuntime(ctx context.Context, cfg config.Config) (*SubscriptionRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	version, err := selectPHPFPM(ctx, cfg, system.OSFS{}, system.CommandSystemd{Commander: commander})
	if err != nil {
		_ = repository.Close()
		return nil, err
	}
	return &SubscriptionRuntime{
		Service:    SubscriptionService{FS: system.OSFS{}, Users: system.CommandUsers{Commander: commander}, Store: repository, PHPVersion: version.Version, Config: cfg},
		repository: repository,
	}, nil
}

func (runtime *SubscriptionRuntime) Close() error { return runtime.repository.Close() }

func (service SubscriptionService) Create(ctx context.Context, name string) (int64, error) {
	return service.CreateWithOptions(ctx, name, SubscriptionCreateOptions{})
}

func (service SubscriptionService) CreateWithOptions(ctx context.Context, name string, options SubscriptionCreateOptions) (int64, error) {
	operation, err := service.PrepareCreateWithOptions(ctx, name, options)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service SubscriptionService) Delete(ctx context.Context, name string, force bool) (int64, error) {
	operation, err := service.PrepareDelete(ctx, name, force)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

func (service SubscriptionService) SetStatus(ctx context.Context, name, status string) (int64, error) {
	if status != "active" && status != "suspended" {
		return 0, fmt.Errorf("unsupported subscription status %q", status)
	}
	subscription, err := service.Show(ctx, name)
	if err != nil {
		return 0, err
	}
	if subscription.Status == status {
		return 0, fmt.Errorf("subscription %q is already %s", name, status)
	}
	if subscription.Status == "archived" {
		return 0, fmt.Errorf("subscription %q is archived", name)
	}
	previous := subscription.Status
	operation := plan.Plan{Action: "subscription.set-status", Target: name, Steps: []plan.Step{{Name: "record subscription status", Preview: "set subscription status to " + status, Do: func(ctx context.Context) error {
		return service.Store.SetSubscriptionStatus(ctx, subscription.ID, status)
	}, Undo: func(ctx context.Context) error {
		return service.Store.SetSubscriptionStatus(ctx, subscription.ID, previous)
	}}}}
	return service.Executor.Run(ctx, operation)
}

// PrepareDelete verifies all destructive targets without changing the system.
func (service SubscriptionService) PrepareDelete(ctx context.Context, name string, force bool) (plan.Plan, error) {
	subscription, err := service.Show(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}
	if subscription.Status != "archived" && !force {
		return plan.Plan{}, fmt.Errorf("subscription %q is %s; archive it first or use --force", name, subscription.Status)
	}
	if err := service.validateDeletionTarget(subscription); err != nil {
		return plan.Plan{}, err
	}
	account, err := service.Users.Lookup(subscription.UnixUser)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("look up subscription user %q: %w", subscription.UnixUser, err)
	}
	if account.Uid != strconv.Itoa(subscription.UnixUID) || filepath.Clean(account.HomeDir) != filepath.Clean(subscription.Home) {
		return plan.Plan{}, fmt.Errorf("unix user %q does not match stored subscription identity", subscription.UnixUser)
	}
	return service.deletePlan(subscription), nil
}

func (service SubscriptionService) validateDeletionTarget(subscription domain.Subscription) error {
	root := filepath.Clean(service.Config.Paths.VHosts)
	expectedHome := filepath.Join(root, subscription.Name)
	if root == "." || root == string(filepath.Separator) || filepath.Clean(subscription.Home) != expectedHome {
		return fmt.Errorf("refuse to delete unexpected subscription home %q", subscription.Home)
	}
	resolvedRoot, err := service.FS.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve vhosts root: %w", err)
	}
	resolvedHome, err := service.FS.EvalSymlinks(subscription.Home)
	if err != nil {
		return fmt.Errorf("resolve subscription home: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedHome)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("refuse to delete home outside vhosts root: %q", subscription.Home)
	}
	if _, err := service.FS.Stat(subscription.Home); err != nil {
		return fmt.Errorf("inspect subscription home %q: %w", subscription.Home, err)
	}
	return nil
}

// PrepareCreate validates the request and reads current state without changing it.
func (service SubscriptionService) PrepareCreate(ctx context.Context, name string) (plan.Plan, error) {
	return service.PrepareCreateWithOptions(ctx, name, SubscriptionCreateOptions{})
}

func (service SubscriptionService) PrepareCreateWithOptions(ctx context.Context, name string, options SubscriptionCreateOptions) (plan.Plan, error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return plan.Plan{}, err
	}
	if options.QuotaDiskBytes < 0 || options.QuotaWebsites < 0 || options.QuotaDatabases < 0 || options.QuotaBackups < 0 {
		return plan.Plan{}, errors.New("quotas must not be negative")
	}
	exists, err := service.Store.SubscriptionExists(ctx, name)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("check subscription: %w", err)
	}
	if exists {
		return plan.Plan{}, fmt.Errorf("subscription %q already exists", name)
	}
	home := filepath.Join(service.Config.Paths.VHosts, name)
	if _, err := service.FS.Stat(home); err == nil {
		return plan.Plan{}, fmt.Errorf("subscription home %q already exists", home)
	} else if !errors.Is(err, os.ErrNotExist) {
		return plan.Plan{}, fmt.Errorf("inspect subscription home: %w", err)
	}
	uid, err := service.nextUID(ctx, name)
	if err != nil {
		return plan.Plan{}, err
	}
	phpVersion := service.PHPVersion
	if phpVersion == "" {
		phpVersion = service.Config.PHP.DefaultVersion
	}
	subscription := domain.Subscription{Name: name, UnixUser: name, UnixUID: uid, Home: home, PHPVersion: phpVersion, PHPMaxChildren: service.Config.PHP.MaxChildren, PHPMemoryLimit: service.Config.PHP.MemoryLimit, PHPUploadMax: service.Config.PHP.UploadMax, PHPMaxExecTime: service.Config.PHP.MaxExecTime, SSHAccess: "none", QuotaDiskBytes: options.QuotaDiskBytes, QuotaWebsites: options.QuotaWebsites, QuotaDatabases: options.QuotaDatabases, QuotaBackups: options.QuotaBackups}
	return service.createPlan(subscription), nil
}

func (service SubscriptionService) nextUID(ctx context.Context, name string) (int, error) {
	_, err := service.Users.Lookup(name)
	if err == nil {
		return 0, fmt.Errorf("unix user %q already exists", name)
	}
	if !isUnknownUser(err) {
		return 0, fmt.Errorf("look up unix user %q: %w", name, err)
	}
	for uid := service.Config.Users.UIDMin; uid <= service.Config.Users.UIDMax; uid++ {
		reserved, err := service.Store.SubscriptionUIDExists(ctx, uid)
		if err != nil {
			return 0, fmt.Errorf("check subscription UID %d: %w", uid, err)
		}
		if reserved {
			continue
		}
		_, err = service.Users.LookupID(strconv.Itoa(uid))
		if err == nil {
			continue
		}
		if !isUnknownUser(err) {
			return 0, fmt.Errorf("look up uid %d: %w", uid, err)
		}
		return uid, nil
	}
	return 0, errors.New("no free UID in configured range")
}

func isUnknownUser(err error) bool {
	var unknownName user.UnknownUserError
	var unknownID user.UnknownUserIdError
	return errors.As(err, &unknownName) || errors.As(err, &unknownID)
}

func (service SubscriptionService) createPlan(subscription domain.Subscription) plan.Plan {
	directories := []struct {
		name, path string
		mode       os.FileMode
	}{
		{"create subscription home", subscription.Home, 0o751},
		{"create sites directory", filepath.Join(subscription.Home, "sites"), 0o751},
		{"create temporary directory", filepath.Join(subscription.Home, "tmp"), 0o700},
		{"create session directory", filepath.Join(subscription.Home, "tmp", "sessions"), 0o700},
		{"create private directory", filepath.Join(subscription.Home, "private"), 0o700},
		{"create SSH directory", filepath.Join(subscription.Home, ".ssh"), 0o700},
	}
	steps := []plan.Step{{Name: "create Unix user", Preview: fmt.Sprintf("/usr/sbin/useradd --uid %d --home %s --shell %s --user-group --no-create-home %s", subscription.UnixUID, subscription.Home, meta.NoLoginShell, subscription.UnixUser), Do: func(ctx context.Context) error {
		return service.Users.Create(ctx, system.CreateUserOptions{Name: subscription.UnixUser, UID: subscription.UnixUID, Home: subscription.Home, Shell: meta.NoLoginShell, UserGroup: true, NoCreateHome: true})
	}, Undo: func(ctx context.Context) error { return service.Users.Delete(ctx, subscription.UnixUser, false) }}}
	for _, directory := range directories {
		directory := directory
		preview := fmt.Sprintf("mkdir -m %04o %s; chown %d:%d %s", directory.mode, directory.path, subscription.UnixUID, subscription.UnixUID, directory.path)
		steps = append(steps, plan.Step{Name: directory.name, Preview: preview, Do: service.createDirectory(directory.path, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "record subscription", Preview: "insert subscription into SQLite", Do: func(ctx context.Context) error { return service.Store.CreateSubscription(ctx, subscription) }, Undo: func(ctx context.Context) error { return service.Store.DeleteSubscription(ctx, subscription.Name) }})
	steps = append(steps, plan.Step{Name: "lock subscription password", Preview: "/usr/sbin/usermod --lock " + subscription.UnixUser, Do: func(ctx context.Context) error {
		return service.Users.LockPassword(ctx, subscription.UnixUser)
	}})
	return plan.Plan{Action: "subscription.create", Target: subscription.Name, Steps: steps}
}

func (service SubscriptionService) deletePlan(subscription domain.Subscription) plan.Plan {
	steps := []plan.Step{
		{Name: "remove subscription home", Preview: fmt.Sprintf("remove recursively %s", subscription.Home), Do: func(context.Context) error { return service.FS.RemoveAll(subscription.Home) }},
		{Name: "delete Unix user", Preview: fmt.Sprintf("/usr/sbin/userdel %s", subscription.UnixUser), Do: func(ctx context.Context) error { return service.Users.Delete(ctx, subscription.UnixUser, false) }},
		{Name: "delete subscription record", Preview: "delete subscription from SQLite", Do: func(ctx context.Context) error { return service.Store.DeleteSubscription(ctx, subscription.Name) }},
	}
	return plan.Plan{Action: "subscription.delete", Target: subscription.Name, Steps: steps}
}

func (service SubscriptionService) createDirectory(path string, uid int, mode os.FileMode) func(context.Context) error {
	return func(context.Context) error {
		if err := service.FS.MkdirAll(path, mode); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if err := service.FS.Chown(path, uid, uid); err != nil {
			_ = service.FS.Remove(path)
			return fmt.Errorf("own %s: %w", path, err)
		}
		if err := service.FS.Chmod(path, mode); err != nil {
			_ = service.FS.Remove(path)
			return fmt.Errorf("set permissions on %s: %w", path, err)
		}
		return nil
	}
}
