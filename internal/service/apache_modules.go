package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"provctl/internal/system"
)

var RequiredApacheModules = []string{"proxy", "proxy_fcgi", "proxy_http", "socache_shmcb", "ssl", "rewrite", "headers"}

// ApacheModules enables Debian Apache modules through their managed symlinks.
// It deliberately avoids a2enmod so the exact filesystem changes are known.
type ApacheModules struct {
	FS            system.FS
	AvailablePath string
	EnabledPath   string
}

// NeedsEnableRequired reports whether a managed module symlink is missing.
// It also validates all existing links before any mutation is attempted.
func (modules ApacheModules) NeedsEnableRequired() (bool, error) {
	if modules.FS == nil || modules.AvailablePath == "" || modules.EnabledPath == "" {
		return false, errors.New("Apache module manager requires filesystem and module paths")
	}
	needed := false
	for _, module := range RequiredApacheModules {
		for _, suffix := range []string{".load", ".conf"} {
			source := filepath.Join(modules.AvailablePath, module+suffix)
			if _, err := modules.FS.Stat(source); err != nil {
				if errors.Is(err, os.ErrNotExist) && suffix == ".conf" {
					continue
				}
				return false, fmt.Errorf("required Apache module file %q: %w", source, err)
			}
			destination := filepath.Join(modules.EnabledPath, filepath.Base(source))
			if _, err := modules.FS.Stat(destination); err == nil {
				resolved, resolveErr := modules.FS.EvalSymlinks(destination)
				if resolveErr != nil || filepath.Clean(resolved) != filepath.Clean(source) {
					return false, fmt.Errorf("refuse to replace existing Apache module link %q", destination)
				}
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return false, fmt.Errorf("inspect Apache module link %q: %w", destination, err)
			}
			needed = true
		}
	}
	return needed, nil
}

// EnableRequired enables each available .load and .conf file for the required modules.
// The undo function removes only symlinks created by this call.
func (modules ApacheModules) EnableRequired(ctx context.Context) (func(context.Context) error, bool, error) {
	if modules.FS == nil || modules.AvailablePath == "" || modules.EnabledPath == "" {
		return nil, false, errors.New("Apache module manager requires filesystem and module paths")
	}
	created := make([]string, 0)
	for _, module := range RequiredApacheModules {
		for _, suffix := range []string{".load", ".conf"} {
			source := filepath.Join(modules.AvailablePath, module+suffix)
			if _, err := modules.FS.Stat(source); err != nil {
				if errors.Is(err, os.ErrNotExist) && suffix == ".conf" {
					continue
				}
				return nil, false, fmt.Errorf("required Apache module file %q: %w", source, err)
			}
			destination := filepath.Join(modules.EnabledPath, filepath.Base(source))
			if _, err := modules.FS.Stat(destination); err == nil {
				resolved, resolveErr := modules.FS.EvalSymlinks(destination)
				if resolveErr != nil || filepath.Clean(resolved) != filepath.Clean(source) {
					return nil, false, fmt.Errorf("refuse to replace existing Apache module link %q", destination)
				}
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, false, fmt.Errorf("inspect Apache module link %q: %w", destination, err)
			}
			if err := modules.FS.Symlink(source, destination); err != nil {
				for index := len(created) - 1; index >= 0; index-- {
					_ = modules.FS.Remove(created[index])
				}
				return nil, false, fmt.Errorf("enable Apache module %q: %w", module, err)
			}
			created = append(created, destination)
		}
	}
	undo := func(context.Context) error {
		var failures []error
		for index := len(created) - 1; index >= 0; index-- {
			if err := modules.FS.Remove(created[index]); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Errorf("remove Apache module link %q: %w", created[index], err))
			}
		}
		return errors.Join(failures...)
	}
	return undo, len(created) > 0, nil
}
