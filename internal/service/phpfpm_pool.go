package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"provctl/internal/system"
)

// PHPFPMPoolApplier applies one PHP-FPM pool and returns an undo operation.
type PHPFPMPoolApplier interface {
	ApplyPool(context.Context, PHPFPMVersion, string, []byte, string) (func(context.Context) error, error)
	RemovePool(context.Context, PHPFPMVersion, string) (func(context.Context) error, error)
}

// RemovePool validates and reloads the old FPM service after removing one
// managed pool. Its undo operation restores the exact previous pool.
func (fpm PHPFPM) RemovePool(ctx context.Context, version PHPFPMVersion, path string) (func(context.Context) error, error) {
	if fpm.FS == nil || fpm.Commands == nil || fpm.Systemd == nil || version.Binary == "" || version.Service == "" || path == "" {
		return nil, errors.New("PHP-FPM pool removal requires filesystem, commander, systemd, version, and path")
	}
	previous, existed, err := fpm.readPrevious(path)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, fmt.Errorf("PHP-FPM pool %q does not exist", path)
	}
	if err := fpm.FS.Remove(path); err != nil {
		return nil, fmt.Errorf("remove PHP-FPM pool %q: %w", path, err)
	}
	restore := func(restoreCtx context.Context) error {
		return fpm.restore(restoreCtx, version, path, previous, true)
	}
	if err := fpm.configTest(ctx, version, "removed configuration"); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(err, restoreErr)
		}
		return nil, err
	}
	if err := fpm.Systemd.Reload(ctx, version.Service); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(fmt.Errorf("reload PHP-FPM: %w", err), restoreErr)
		}
		return nil, fmt.Errorf("reload PHP-FPM: %w", err)
	}
	active, err := fpm.Systemd.IsActive(ctx, version.Service)
	if err != nil {
		return nil, fmt.Errorf("check PHP-FPM service: %w", err)
	}
	if !active {
		return nil, fmt.Errorf("PHP-FPM service %q is not active after reload", version.Service)
	}
	return restore, nil
}

// PHPFPM applies a pool only after validating the PHP-FPM configuration.
type PHPFPM struct {
	FS       system.FS
	Commands system.Commander
	Systemd  system.Systemd
}

func (fpm PHPFPM) ApplyPool(ctx context.Context, version PHPFPMVersion, path string, contents []byte, socket string) (func(context.Context) error, error) {
	if fpm.FS == nil || fpm.Commands == nil || fpm.Systemd == nil || version.Binary == "" || version.Service == "" || path == "" || socket == "" {
		return nil, errors.New("PHP-FPM pool requires filesystem, commander, systemd, version, path, and socket")
	}
	previous, existed, err := fpm.readPrevious(path)
	if err != nil {
		return nil, err
	}
	if err := fpm.FS.WriteFileAtomic(path, contents, 0o640); err != nil {
		return nil, fmt.Errorf("write PHP-FPM pool %q: %w", path, err)
	}
	restore := func(restoreCtx context.Context) error {
		return fpm.restore(restoreCtx, version, path, previous, existed)
	}
	if err := fpm.configTest(ctx, version, "updated configuration"); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(err, restoreErr)
		}
		return nil, err
	}
	if err := fpm.Systemd.Reload(ctx, version.Service); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(fmt.Errorf("reload PHP-FPM: %w", err), restoreErr)
		}
		return nil, fmt.Errorf("reload PHP-FPM: %w", err)
	}
	if err := fpm.verify(ctx, version, socket); err != nil {
		if restoreErr := restore(ctx); restoreErr != nil {
			return nil, errors.Join(err, restoreErr)
		}
		return nil, err
	}
	return restore, nil
}

func (fpm PHPFPM) readPrevious(path string) ([]byte, bool, error) {
	_, err := fpm.FS.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect PHP-FPM pool %q: %w", path, err)
	}
	contents, err := fpm.FS.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read PHP-FPM pool %q: %w", path, err)
	}
	return contents, true, nil
}

func (fpm PHPFPM) restore(ctx context.Context, version PHPFPMVersion, path string, previous []byte, existed bool) error {
	if existed {
		if err := fpm.FS.WriteFileAtomic(path, previous, 0o640); err != nil {
			return fmt.Errorf("restore PHP-FPM pool %q: %w", path, err)
		}
	} else if err := fpm.FS.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove PHP-FPM pool %q: %w", path, err)
	}
	if err := fpm.configTest(ctx, version, "restored configuration"); err != nil {
		return err
	}
	if err := fpm.Systemd.Reload(ctx, version.Service); err != nil {
		return fmt.Errorf("reload PHP-FPM after restore: %w", err)
	}
	return nil
}

func (fpm PHPFPM) configTest(ctx context.Context, version PHPFPMVersion, phase string) error {
	result, err := fpm.Commands.Run(ctx, version.Binary, "-t")
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if output == "" {
		return fmt.Errorf("PHP-FPM %s configtest: %w", phase, err)
	}
	return fmt.Errorf("PHP-FPM %s configtest: %w: %s", phase, err, output)
}

func (fpm PHPFPM) verify(ctx context.Context, version PHPFPMVersion, socket string) error {
	active, err := fpm.Systemd.IsActive(ctx, version.Service)
	if err != nil {
		return fmt.Errorf("check PHP-FPM service: %w", err)
	}
	if !active {
		return fmt.Errorf("PHP-FPM service %q is not active after reload", version.Service)
	}
	deadline := time.Now().Add(5 * time.Second)
	var info os.FileInfo
	for {
		info, err = fpm.FS.Stat(socket)
		if err == nil || time.Now().After(deadline) {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err != nil {
		return fmt.Errorf("inspect PHP-FPM socket %q: %w", socket, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("PHP-FPM path %q is not a socket", socket)
	}
	return nil
}
