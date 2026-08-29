package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/render"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

type BootstrapService struct {
	FS          system.FS
	Modules     ApacheModules
	Certificate DefaultCertificate
	Apache      ApacheVHostApplier
	Executor    plan.Executor
	Config      config.Config
}
type BootstrapRuntime struct {
	Service    BootstrapService
	repository *sqlite.Repository
}

func NewProductionBootstrapRuntime(ctx context.Context, cfg config.Config) (*BootstrapRuntime, error) {
	fs := system.OSFS{}
	if err := fs.MkdirAll(meta.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("initialize state directory: %w", err)
	}
	if err := fs.Chmod(meta.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure state directory: %w", err)
	}
	repository, err := sqlite.Open(ctx, meta.DatabaseFile)
	if err != nil {
		return nil, err
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return &BootstrapRuntime{Service: BootstrapService{FS: fs, Modules: ApacheModules{FS: fs, AvailablePath: "/etc/apache2/mods-available", EnabledPath: "/etc/apache2/mods-enabled"}, Certificate: DefaultCertificate{FS: fs, Commands: commander, Directory: meta.DefaultSSLDir, Certificate: meta.DefaultSSLCertificate, Key: meta.DefaultSSLKey}, Apache: Apache{FS: fs, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service}, Executor: plan.Executor{Journal: sqlite.OperationJournal{DB: repository.DB}, Locker: system.FileLocker{Path: meta.LockFile}}, Config: cfg}, repository: repository}, nil
}
func (runtime *BootstrapRuntime) Close() error { return runtime.repository.Close() }
func NewBootstrapPreview(cfg config.Config) BootstrapService {
	fs := system.OSFS{}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return BootstrapService{FS: fs, Modules: ApacheModules{FS: fs, AvailablePath: "/etc/apache2/mods-available", EnabledPath: "/etc/apache2/mods-enabled"}, Certificate: DefaultCertificate{FS: fs, Commands: commander, Directory: meta.DefaultSSLDir, Certificate: meta.DefaultSSLCertificate, Key: meta.DefaultSSLKey}, Apache: Apache{FS: fs, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service}, Config: cfg}
}
func (service BootstrapService) Run(ctx context.Context) (int64, error) {
	operation, err := service.Prepare(ctx)
	if err != nil {
		return 0, err
	}
	return service.Executor.Run(ctx, operation)
}
func (service BootstrapService) Prepare(ctx context.Context) (plan.Plan, error) {
	contents, err := render.RenderDefaultApacheVHost(render.DefaultApacheVHost{CertificateFile: meta.DefaultSSLCertificate, KeyFile: meta.DefaultSSLKey})
	if err != nil {
		return plan.Plan{}, err
	}
	vhost := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+"000-default.conf")
	enabled := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhost))
	var undoModules, undoCertificate func(context.Context) error
	steps := []plan.Step{{Name: "enable required Apache modules", Preview: "enable managed Apache module symlinks", Do: func(ctx context.Context) error {
		var err error
		undoModules, _, err = service.Modules.EnableRequired(ctx)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoModules == nil {
			return nil
		}
		return undoModules(ctx)
	}}, {Name: "create default TLS certificate", Preview: "generate self-signed catch-all certificate", Do: func(ctx context.Context) error {
		var err error
		undoCertificate, _, err = service.Certificate.Ensure(ctx)
		return err
	}, Undo: func(ctx context.Context) error {
		if undoCertificate == nil {
			return nil
		}
		return undoCertificate(ctx)
	}}}
	installed, err := defaultVHostInstalled(service.FS, vhost, enabled, contents)
	if err != nil {
		return plan.Plan{}, err
	}
	if !installed {
		var undo func(context.Context) error
		steps = append(steps, plan.Step{Name: "install default Apache vhost", Preview: "write and enable " + vhost, Do: func(ctx context.Context) error {
			var err error
			undo, err = service.Apache.ApplyVHost(ctx, vhost, contents, enabled)
			return err
		}, Undo: func(ctx context.Context) error {
			if undo == nil {
				return nil
			}
			return undo(ctx)
		}})
	}
	return plan.Plan{Action: "bootstrap", Target: "server", Steps: steps}, nil
}
func defaultVHostInstalled(fs system.FS, path, enabled string, contents []byte) (bool, error) {
	current, err := fs.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !bytes.Equal(current, contents) {
		return false, fmt.Errorf("refuse to replace existing default vhost %q", path)
	}
	target, err := fs.EvalSymlinks(enabled)
	if err != nil {
		return false, fmt.Errorf("inspect default vhost link: %w", err)
	}
	return filepath.Clean(target) == filepath.Clean(path), nil
}
