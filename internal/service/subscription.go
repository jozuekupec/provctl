package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

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
	CreateSubscription(context.Context, domain.Subscription) error
	DeleteSubscription(context.Context, string) error
}

type SubscriptionService struct {
	FS       system.FS
	Users    system.Users
	Store    SubscriptionStore
	Executor plan.Executor
	Config   config.Config
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
	return &SubscriptionRuntime{
		Service: SubscriptionService{
			FS:    system.OSFS{},
			Users: system.CommandUsers{Commander: system.ExecCommander{}},
			Store: repository,
			Executor: plan.Executor{
				Journal: sqlite.OperationJournal{DB: repository.DB},
				Locker:  system.FileLocker{Path: meta.LockFile},
			},
			Config: cfg,
		},
		repository: repository,
	}, nil
}

func NewReadOnlySubscriptionRuntime(ctx context.Context, cfg config.Config) (*SubscriptionRuntime, error) {
	repository, err := sqlite.OpenReadOnly(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	return &SubscriptionRuntime{
		Service:    SubscriptionService{FS: system.OSFS{}, Users: system.CommandUsers{Commander: system.ExecCommander{}}, Store: repository, Config: cfg},
		repository: repository,
	}, nil
}

func (runtime *SubscriptionRuntime) Close() error { return runtime.repository.Close() }

func (service SubscriptionService) Create(ctx context.Context, name string) (int64, error) {
	operation, err := service.PrepareCreate(ctx, name)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}

// PrepareCreate validates the request and reads current state without changing it.
func (service SubscriptionService) PrepareCreate(ctx context.Context, name string) (plan.Plan, error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return plan.Plan{}, err
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
	subscription := domain.Subscription{Name: name, UnixUser: name, UnixUID: uid, Home: home, PHPVersion: service.Config.PHP.DefaultVersion, PHPMaxChildren: service.Config.PHP.MaxChildren, PHPMemoryLimit: service.Config.PHP.MemoryLimit, PHPUploadMax: service.Config.PHP.UploadMax, PHPMaxExecTime: service.Config.PHP.MaxExecTime, SSHAccess: "none"}
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
	steps := []plan.Step{{Name: "create Unix user", Preview: fmt.Sprintf("/usr/sbin/useradd --uid %d --home %s --shell %s --user-group --no-create-home %s", subscription.UnixUID, subscription.Home, service.Config.Users.Shell, subscription.UnixUser), Do: func(ctx context.Context) error {
		return service.Users.Create(ctx, system.CreateUserOptions{Name: subscription.UnixUser, UID: subscription.UnixUID, Home: subscription.Home, Shell: service.Config.Users.Shell, UserGroup: true, NoCreateHome: true})
	}, Undo: func(ctx context.Context) error { return service.Users.Delete(ctx, subscription.UnixUser, false) }}}
	for _, directory := range directories {
		directory := directory
		preview := fmt.Sprintf("mkdir -m %04o %s; chown %d:%d %s", directory.mode, directory.path, subscription.UnixUID, subscription.UnixUID, directory.path)
		steps = append(steps, plan.Step{Name: directory.name, Preview: preview, Do: service.createDirectory(directory.path, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "record subscription", Preview: "insert subscription into SQLite", Do: func(ctx context.Context) error { return service.Store.CreateSubscription(ctx, subscription) }, Undo: func(ctx context.Context) error { return service.Store.DeleteSubscription(ctx, subscription.Name) }})
	return plan.Plan{Action: "subscription.create", Target: subscription.Name, Steps: steps}
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
