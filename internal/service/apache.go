package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"provctl/internal/system"
)

// Apache applies a single managed configuration file with validation and rollback.
type Apache struct {
	FS       system.FS
	Commands system.Commander
	Systemd  system.Systemd
	Service  string
}

// ValidateAndReload validates the complete Apache configuration and reloads it.
func (apache Apache) ValidateAndReload(ctx context.Context) error {
	if apache.FS == nil || apache.Commands == nil || apache.Systemd == nil || apache.Service == "" {
		return errors.New("Apache service requires filesystem, commander, systemd, and service name")
	}
	if err := apache.configTest(ctx, "updated configuration"); err != nil {
		return err
	}
	if err := apache.Systemd.Reload(ctx, apache.Service); err != nil {
		return fmt.Errorf("reload Apache: %w", err)
	}
	active, err := apache.Systemd.IsActive(ctx, apache.Service)
	if err != nil {
		return fmt.Errorf("check Apache service: %w", err)
	}
	if !active {
		return fmt.Errorf("Apache service %q is not active after reload", apache.Service)
	}
	return nil
}

// Apply writes a configuration file only after the existing Apache
// configuration passes a baseline test. The returned function restores the
// exact previous file state and reloads Apache, making it suitable as a plan
// rollback step.
func (apache Apache) Apply(ctx context.Context, path string, contents []byte) (func(context.Context) error, error) {
	return apache.apply(ctx, path, contents, "")
}

// ApplyVHost installs a new managed vhost and enables it by a direct symlink.
// It refuses to replace an existing enabled path.
func (apache Apache) ApplyVHost(ctx context.Context, path string, contents []byte, enabledPath string) (func(context.Context) error, error) {
	if enabledPath == "" {
		return nil, errors.New("Apache enabled vhost path is required")
	}
	if _, err := apache.FS.Stat(enabledPath); err == nil {
		return nil, fmt.Errorf("refuse to replace existing enabled Apache vhost %q", enabledPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect enabled Apache vhost %q: %w", enabledPath, err)
	}
	return apache.apply(ctx, path, contents, enabledPath)
}

// SetVHostEnabled enables or disables an existing managed vhost. The returned
// function restores the prior symlink state and reloads Apache.
func (apache Apache) SetVHostEnabled(ctx context.Context, path, enabledPath string, enabled bool) (func(context.Context) error, error) {
	if apache.FS == nil || apache.Commands == nil || apache.Systemd == nil || apache.Service == "" {
		return nil, errors.New("Apache service requires filesystem, commander, systemd, and service name")
	}
	if enabledPath == "" {
		return nil, errors.New("Apache enabled vhost path is required")
	}
	if _, err := apache.FS.Stat(enabledPath); enabled == (err == nil) {
		return nil, fmt.Errorf("Apache vhost %q is already enabled=%t", enabledPath, enabled)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect enabled Apache vhost %q: %w", enabledPath, err)
	}
	if enabled {
		if _, err := apache.FS.Stat(path); err != nil {
			return nil, fmt.Errorf("inspect Apache vhost %q: %w", path, err)
		}
	}
	if err := apache.configTest(ctx, "baseline"); err != nil {
		return nil, err
	}
	apply := func() error {
		if enabled {
			return apache.FS.Symlink(path, enabledPath)
		}
		return apache.FS.Remove(enabledPath)
	}
	restore := func(restoreCtx context.Context) error {
		if enabled {
			if err := apache.FS.Remove(enabledPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove enabled Apache vhost %q: %w", enabledPath, err)
			}
		} else if err := apache.FS.Symlink(path, enabledPath); err != nil {
			return fmt.Errorf("restore enabled Apache vhost %q: %w", enabledPath, err)
		}
		return apache.ValidateAndReload(restoreCtx)
	}
	if err := apply(); err != nil {
		return nil, fmt.Errorf("set Apache vhost enabled=%t: %w", enabled, err)
	}
	if err := apache.ValidateAndReload(ctx); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(err, restoreErr)
		}
		return nil, err
	}
	return restore, nil
}

func (apache Apache) apply(ctx context.Context, path string, contents []byte, enabledPath string) (func(context.Context) error, error) {
	if apache.FS == nil || apache.Commands == nil || apache.Systemd == nil || apache.Service == "" {
		return nil, errors.New("Apache service requires filesystem, commander, systemd, and service name")
	}
	if err := apache.configTest(ctx, "baseline"); err != nil {
		return nil, err
	}
	previous, existed, err := apache.readPrevious(path)
	if err != nil {
		return nil, err
	}
	if err := apache.FS.WriteFileAtomic(path, contents, 0o640); err != nil {
		return nil, fmt.Errorf("write Apache configuration %q: %w", path, err)
	}
	restore := func(restoreCtx context.Context) error {
		if enabledPath != "" {
			if err := apache.FS.Remove(enabledPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove enabled Apache vhost %q: %w", enabledPath, err)
			}
		}
		if err := apache.restore(restoreCtx, path, previous, existed); err != nil {
			return err
		}
		return nil
	}
	if enabledPath != "" {
		if err := apache.FS.Symlink(path, enabledPath); err != nil {
			_ = restore(ctx)
			return nil, fmt.Errorf("enable Apache vhost %q: %w", enabledPath, err)
		}
	}
	if err := apache.configTest(ctx, "updated configuration"); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(err, restoreErr)
		}
		return nil, err
	}
	if err := apache.Systemd.Reload(ctx, apache.Service); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(fmt.Errorf("reload Apache: %w", err), restoreErr)
		}
		return nil, fmt.Errorf("reload Apache: %w", err)
	}
	active, err := apache.Systemd.IsActive(ctx, apache.Service)
	if err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(fmt.Errorf("check Apache service: %w", err), restoreErr)
		}
		return nil, fmt.Errorf("check Apache service: %w", err)
	}
	if !active {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(fmt.Errorf("Apache service %q is not active after reload", apache.Service), restoreErr)
		}
		return nil, fmt.Errorf("Apache service %q is not active after reload", apache.Service)
	}
	return restore, nil
}

func (apache Apache) readPrevious(path string) ([]byte, bool, error) {
	_, err := apache.FS.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect Apache configuration %q: %w", path, err)
	}
	contents, err := apache.FS.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read Apache configuration %q: %w", path, err)
	}
	return contents, true, nil
}

func (apache Apache) restore(ctx context.Context, path string, previous []byte, existed bool) error {
	if existed {
		if err := apache.FS.WriteFileAtomic(path, previous, 0o640); err != nil {
			return fmt.Errorf("restore Apache configuration %q: %w", path, err)
		}
	} else if err := apache.FS.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Apache configuration %q: %w", path, err)
	}
	if err := apache.configTest(ctx, "restored configuration"); err != nil {
		return err
	}
	if err := apache.Systemd.Reload(ctx, apache.Service); err != nil {
		return fmt.Errorf("reload Apache after restore: %w", err)
	}
	return nil
}

func (apache Apache) configTest(ctx context.Context, phase string) error {
	result, err := apache.Commands.Run(ctx, "/usr/sbin/apachectl", "configtest")
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if output == "" {
		return fmt.Errorf("Apache %s configtest: %w", phase, err)
	}
	return fmt.Errorf("Apache %s configtest: %w: %s", phase, err, output)
}
