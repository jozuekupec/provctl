package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/plan"
	"provctl/internal/system"
)

type SubscriptionStore interface {
	SubscriptionExists(context.Context, string) (bool, error)
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

func (service SubscriptionService) Create(ctx context.Context, name string) (int64, error) {
	if err := domain.ValidateSubscriptionName(name); err != nil {
		return 0, err
	}
	exists, err := service.Store.SubscriptionExists(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("check subscription: %w", err)
	}
	if exists {
		return 0, fmt.Errorf("subscription %q already exists", name)
	}
	home := filepath.Join(service.Config.Paths.VHosts, name)
	if _, err := service.FS.Stat(home); err == nil {
		return 0, fmt.Errorf("subscription home %q already exists", home)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect subscription home: %w", err)
	}
	uid, err := service.nextUID(name)
	if err != nil {
		return 0, err
	}
	subscription := domain.Subscription{Name: name, UnixUser: name, UnixUID: uid, Home: home, PHPVersion: service.Config.PHP.DefaultVersion, PHPMaxChildren: service.Config.PHP.MaxChildren, PHPMemoryLimit: service.Config.PHP.MemoryLimit, PHPUploadMax: service.Config.PHP.UploadMax, PHPMaxExecTime: service.Config.PHP.MaxExecTime, SSHAccess: "none"}
	return service.Executor.Run(ctx, service.createPlan(subscription))
}

func (service SubscriptionService) nextUID(name string) (int, error) {
	for uid := service.Config.Users.UIDMin; uid <= service.Config.Users.UIDMax; uid++ {
		_, err := service.Users.Lookup(name)
		if err == nil {
			return 0, fmt.Errorf("unix user %q already exists", name)
		}
		if !isUnknownUser(err) {
			return 0, fmt.Errorf("look up unix user %q: %w", name, err)
		}
		_, err = service.Users.LookupID(fmt.Sprint(uid))
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
	steps := []plan.Step{{Name: "create Unix user", Do: func(ctx context.Context) error {
		return service.Users.Create(ctx, system.CreateUserOptions{Name: subscription.UnixUser, UID: subscription.UnixUID, Home: subscription.Home, Shell: service.Config.Users.Shell, UserGroup: true, NoCreateHome: true})
	}, Undo: func(ctx context.Context) error { return service.Users.Delete(ctx, subscription.UnixUser, false) }}}
	for _, directory := range directories {
		directory := directory
		steps = append(steps, plan.Step{Name: directory.name, Do: service.createDirectory(directory.path, subscription.UnixUID, directory.mode), Undo: func(context.Context) error { return service.FS.Remove(directory.path) }})
	}
	steps = append(steps, plan.Step{Name: "record subscription", Do: func(ctx context.Context) error { return service.Store.CreateSubscription(ctx, subscription) }, Undo: func(ctx context.Context) error { return service.Store.DeleteSubscription(ctx, subscription.Name) }})
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
