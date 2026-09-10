package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Apache      BootstrapApache
	Executor    plan.Executor
	Config      config.Config
	AuditGroup  int
}

type BootstrapApache interface {
	ApacheVHostApplier
	ValidateAndReload(context.Context) error
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
	auditGroup, err := (system.OSGroupLookup{}).LookupGroupID("adm")
	if err != nil {
		return nil, fmt.Errorf("look up audit group: %w", err)
	}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return &BootstrapRuntime{Service: BootstrapService{FS: fs, Modules: ApacheModules{FS: fs, AvailablePath: "/etc/apache2/mods-available", EnabledPath: "/etc/apache2/mods-enabled"}, Certificate: DefaultCertificate{FS: fs, Commands: commander, Directory: meta.DefaultSSLDir, Certificate: meta.DefaultSSLCertificate, Key: meta.DefaultSSLKey}, Apache: Apache{FS: fs, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service}, Executor: productionExecutor(repository), Config: cfg, AuditGroup: auditGroup}, repository: repository}, nil
}
func (runtime *BootstrapRuntime) Close() error { return runtime.repository.Close() }
func NewBootstrapPreview(cfg config.Config) (BootstrapService, error) {
	auditGroup, err := (system.OSGroupLookup{}).LookupGroupID("adm")
	if err != nil {
		return BootstrapService{}, fmt.Errorf("look up audit group: %w", err)
	}
	fs := system.OSFS{}
	commander := system.ExecCommander{}
	systemd := system.CommandSystemd{Commander: commander}
	return BootstrapService{FS: fs, Modules: ApacheModules{FS: fs, AvailablePath: "/etc/apache2/mods-available", EnabledPath: "/etc/apache2/mods-enabled"}, Certificate: DefaultCertificate{FS: fs, Commands: commander, Directory: meta.DefaultSSLDir, Certificate: meta.DefaultSSLCertificate, Key: meta.DefaultSSLKey}, Apache: Apache{FS: fs, Commands: commander, Systemd: systemd, Service: cfg.Apache.Service}, Config: cfg, AuditGroup: auditGroup}, nil
}
func (service BootstrapService) Run(ctx context.Context) (int64, bool, error) {
	return service.RunWithSkip(ctx, nil)
}

// RunWithSkip executes selected optional artifact steps. Directories are never
// skippable because later artifacts depend on their secure ownership.
func (service BootstrapService) RunWithSkip(ctx context.Context, skipped []string) (int64, bool, error) {
	operation, err := service.PrepareWithSkip(ctx, skipped)
	if err != nil {
		return 0, false, err
	}
	if len(operation.Steps) == 0 {
		return 0, true, nil
	}
	id, err := service.Executor.Run(ctx, operation)
	return id, false, err
}

func (service BootstrapService) PrepareWithSkip(ctx context.Context, skipped []string) (plan.Plan, error) {
	operation, err := service.Prepare(ctx)
	if err != nil || len(skipped) == 0 {
		return operation, err
	}
	aliases := map[string]string{
		"modules": "enable required Apache modules", "certificate": "create default TLS certificate",
		"vhost": "install default Apache vhost", "deploy-hook": "install Certbot deploy hook",
		"logrotate": "install audit logrotate configuration", "audit-log": "create audit log",
	}
	wanted := make(map[string]bool, len(skipped))
	for _, name := range skipped {
		step, ok := aliases[name]
		if !ok {
			return plan.Plan{}, fmt.Errorf("unsupported bootstrap skip %q", name)
		}
		wanted[step] = true
	}
	steps := make([]plan.Step, 0, len(operation.Steps))
	for _, step := range operation.Steps {
		if !wanted[step.Name] {
			steps = append(steps, step)
		}
	}
	if len(steps) == 1 && strings.HasPrefix(steps[0].Name, "validate and reload Apache") {
		steps = nil
	}
	operation.Steps = steps
	return operation, nil
}
func (service BootstrapService) Prepare(ctx context.Context) (plan.Plan, error) {
	contents, err := render.RenderDefaultApacheVHost(render.DefaultApacheVHost{CertificateFile: meta.DefaultSSLCertificate, KeyFile: meta.DefaultSSLKey})
	if err != nil {
		return plan.Plan{}, err
	}
	vhost := filepath.Join(service.Config.Apache.SitesAvailable, meta.FilePrefix+"000-default.conf")
	enabled := filepath.Join(service.Config.Apache.SitesEnabled, filepath.Base(vhost))
	if service.AuditGroup < 0 {
		return plan.Plan{}, errors.New("bootstrap audit group ID must not be negative")
	}
	directories := []managedDirectory{
		{path: meta.ConfigDir, mode: 0o755, name: "create configuration directory"},
		{path: meta.StateDir, mode: 0o700, name: "create state directory"},
		{path: service.Config.Paths.ACMEChallenge, mode: 0o755, name: "create ACME challenge directory"},
		{path: filepath.Join(service.Config.Paths.ACMEChallenge, ".well-known", "acme-challenge"), mode: 0o755, name: "create ACME challenge content directory"},
		{path: meta.LogDir, mode: 0o751, name: "create log directory", gid: service.AuditGroup, previousModes: []os.FileMode{0o750}},
		{path: service.Config.Paths.VHosts, mode: 0o755, name: "create vhosts root"},
	}
	steps := make([]plan.Step, 0, 10)
	for _, directory := range directories {
		needed, err := directory.needs(service.FS)
		if err != nil {
			return plan.Plan{}, err
		}
		if needed {
			steps = append(steps, service.managedDirectoryStep(directory))
		}
	}

	modulesNeeded, err := service.Modules.NeedsEnableRequired()
	if err != nil {
		return plan.Plan{}, err
	}
	if modulesNeeded {
		var undoModules func(context.Context) error
		steps = append(steps, plan.Step{Name: "enable required Apache modules", Preview: "enable managed Apache module symlinks", Do: func(ctx context.Context) error {
			var err error
			undoModules, _, err = service.Modules.EnableRequired(ctx)
			return err
		}, Undo: func(ctx context.Context) error {
			if undoModules == nil {
				return nil
			}
			return undoModules(ctx)
		}})
	}
	certificateNeeded, err := defaultCertificateNeeded(service.FS, meta.DefaultSSLCertificate, meta.DefaultSSLKey)
	if err != nil {
		return plan.Plan{}, err
	}
	if certificateNeeded {
		var undoCertificate func(context.Context) error
		steps = append(steps, plan.Step{Name: "create default TLS certificate", Preview: "generate self-signed catch-all certificate", Do: func(ctx context.Context) error {
			var err error
			undoCertificate, _, err = service.Certificate.Ensure(ctx)
			return err
		}, Undo: func(ctx context.Context) error {
			if undoCertificate == nil {
				return nil
			}
			return undoCertificate(ctx)
		}})
	}
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
	hook := []byte("#!/bin/sh\n# GENERATED BY PROVCTL — DO NOT EDIT\n/usr/bin/provctl ssl deploy-hook --lineage \"$RENEWED_LINEAGE\" >> /var/log/provctl/deploy-hook.log 2>&1\nexit 0\n")
	needed, err := managedFileNeeded(service.FS, meta.DeployHook, hook)
	if err != nil {
		return plan.Plan{}, err
	}
	if needed {
		steps = append(steps, service.managedFileStep("install Certbot deploy hook", meta.DeployHook, hook, 0o750))
	}
	logrotate := []byte("/var/log/provctl/*.log /var/log/provctl/*.jsonl {\n    weekly\n    rotate 14\n    missingok\n    notifempty\n    compress\n    delaycompress\n    create 0640 root adm\n}\n")
	needed, err = managedFileNeeded(service.FS, meta.LogrotateConfig, logrotate)
	if err != nil {
		return plan.Plan{}, err
	}
	if needed {
		steps = append(steps, service.managedFileStep("install audit logrotate configuration", meta.LogrotateConfig, logrotate, 0o644))
	}
	auditNeeded, err := fileNeeded(service.FS, meta.AuditLog)
	if err != nil {
		return plan.Plan{}, err
	}
	if auditNeeded {
		steps = append(steps, service.auditLogStep())
	}
	if len(steps) > 0 {
		steps = append(steps, plan.Step{Name: "validate and reload Apache", Preview: "apachectl configtest and reload " + service.Config.Apache.Service, Do: service.Apache.ValidateAndReload})
	}
	return plan.Plan{Action: "bootstrap", Target: "server", Steps: steps}, nil
}

type managedDirectory struct {
	name          string
	path          string
	mode          os.FileMode
	gid           int
	previousModes []os.FileMode
}

func (directory managedDirectory) needs(fs system.FS) (bool, error) {
	info, err := fs.Stat(directory.path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect directory %q: %w", directory.path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("refuse to replace non-directory %q", directory.path)
	}
	if info.Mode().Perm() != directory.mode {
		for _, previousMode := range directory.previousModes {
			if info.Mode().Perm() == previousMode {
				return true, nil
			}
		}
		return false, fmt.Errorf("refuse to change permissions of existing directory %q", directory.path)
	}
	return false, nil
}

func (service BootstrapService) managedDirectoryStep(directory managedDirectory) plan.Step {
	created := false
	var previousMode os.FileMode
	return plan.Step{Name: directory.name, Preview: fmt.Sprintf("mkdir -m %04o %s", directory.mode, directory.path), Do: func(context.Context) error {
		info, err := service.FS.Stat(directory.path)
		if errors.Is(err, os.ErrNotExist) {
			created = true
		} else if err != nil {
			return fmt.Errorf("inspect directory %q before update: %w", directory.path, err)
		} else {
			previousMode = info.Mode().Perm()
		}
		if err := service.FS.MkdirAll(directory.path, directory.mode); err != nil {
			return fmt.Errorf("create directory %q: %w", directory.path, err)
		}
		if err := service.FS.Chown(directory.path, 0, directory.gid); err != nil {
			return fmt.Errorf("own directory %q: %w", directory.path, err)
		}
		if err := service.FS.Chmod(directory.path, directory.mode); err != nil {
			return fmt.Errorf("set directory permissions %q: %w", directory.path, err)
		}
		return nil
	}, Undo: func(context.Context) error {
		if created {
			return service.FS.Remove(directory.path)
		}
		return service.FS.Chmod(directory.path, previousMode)
	}}
}

func (service BootstrapService) managedFileStep(name, path string, contents []byte, mode os.FileMode) plan.Step {
	var created bool
	return plan.Step{Name: name, Preview: "write " + path, Do: func(context.Context) error {
		current, err := service.FS.ReadFile(path)
		if err == nil {
			if bytes.Equal(current, contents) {
				return nil
			}
			return fmt.Errorf("refuse to replace unmanaged file %q", path)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read managed file %q: %w", path, err)
		}
		if err := service.FS.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create parent directory for %q: %w", path, err)
		}
		if err := service.FS.WriteFileAtomic(path, contents, mode); err != nil {
			return fmt.Errorf("write managed file %q: %w", path, err)
		}
		created = true
		return service.FS.Chmod(path, mode)
	}, Undo: func(context.Context) error {
		if !created {
			return nil
		}
		return service.FS.Remove(path)
	}}
}

func (service BootstrapService) auditLogStep() plan.Step {
	created := false
	return plan.Step{Name: "create audit log", Preview: "create " + meta.AuditLog, Do: func(context.Context) error {
		if err := service.FS.MkdirAll(filepath.Dir(meta.AuditLog), 0o750); err != nil {
			return fmt.Errorf("create audit log directory: %w", err)
		}
		if err := service.FS.WriteFileAtomic(meta.AuditLog, nil, 0o640); err != nil {
			return fmt.Errorf("create audit log: %w", err)
		}
		created = true
		if err := service.FS.Chown(meta.AuditLog, 0, service.AuditGroup); err != nil {
			return fmt.Errorf("own audit log: %w", err)
		}
		return service.FS.Chmod(meta.AuditLog, 0o640)
	}, Undo: func(context.Context) error {
		if !created {
			return nil
		}
		return service.FS.Remove(meta.AuditLog)
	}}
}

func defaultCertificateNeeded(fs system.FS, certificate, key string) (bool, error) {
	certificateExists, err := exists(fs, certificate)
	if err != nil {
		return false, err
	}
	keyExists, err := exists(fs, key)
	if err != nil {
		return false, err
	}
	if certificateExists != keyExists {
		return false, errors.New("refuse to replace incomplete default TLS certificate")
	}
	return !certificateExists, nil
}

func managedFileNeeded(fs system.FS, path string, contents []byte) (bool, error) {
	current, err := fs.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read managed file %q: %w", path, err)
	}
	if !bytes.Equal(current, contents) {
		return false, fmt.Errorf("refuse to replace unmanaged file %q", path)
	}
	return false, nil
}

func fileNeeded(fs system.FS, path string) (bool, error) {
	_, err := fs.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect file %q: %w", path, err)
	}
	return false, nil
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
