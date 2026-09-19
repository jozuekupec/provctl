package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/render"
	"provctl/internal/system"
)

type bootstrapEntry struct {
	data []byte
	dir  bool
	mode os.FileMode
}

type bootstrapFS struct {
	entries map[string]bootstrapEntry
	links   map[string]string
}

func (fs *bootstrapFS) Stat(path string) (os.FileInfo, error) {
	entry, ok := fs.entries[path]
	if !ok {
		if _, ok := fs.links[path]; ok {
			return bootstrapInfo{mode: 0o777}, nil
		}
		return nil, os.ErrNotExist
	}
	return bootstrapInfo{mode: entry.mode, dir: entry.dir}, nil
}
func (fs *bootstrapFS) ReadFile(path string) ([]byte, error) {
	entry, ok := fs.entries[path]
	if !ok || entry.dir {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), entry.data...), nil
}
func (fs *bootstrapFS) WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	fs.entries[path] = bootstrapEntry{data: append([]byte(nil), data...), mode: mode}
	return nil
}
func (fs *bootstrapFS) Remove(path string) error {
	delete(fs.entries, path)
	delete(fs.links, path)
	return nil
}
func (fs *bootstrapFS) RemoveAll(path string) error { return fs.Remove(path) }
func (fs *bootstrapFS) MkdirAll(path string, mode os.FileMode) error {
	fs.entries[path] = bootstrapEntry{dir: true, mode: mode}
	return nil
}
func (*bootstrapFS) Chown(string, int, int) error { return nil }
func (fs *bootstrapFS) Chmod(path string, mode os.FileMode) error {
	entry := fs.entries[path]
	entry.mode = mode
	fs.entries[path] = entry
	return nil
}
func (fs *bootstrapFS) Symlink(oldname, newname string) error {
	fs.links[newname] = oldname
	return nil
}
func (*bootstrapFS) ReadDir(string) ([]os.DirEntry, error) { return nil, nil }
func (fs *bootstrapFS) EvalSymlinks(path string) (string, error) {
	target, ok := fs.links[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return target, nil
}

type bootstrapInfo struct {
	mode os.FileMode
	dir  bool
}

func (bootstrapInfo) Name() string { return "entry" }
func (bootstrapInfo) Size() int64  { return 0 }
func (info bootstrapInfo) Mode() os.FileMode {
	if info.dir {
		return os.ModeDir | info.mode
	}
	return info.mode
}
func (bootstrapInfo) ModTime() time.Time { return time.Time{} }
func (info bootstrapInfo) IsDir() bool   { return info.dir }
func (bootstrapInfo) Sys() any           { return nil }

type bootstrapApache struct{}

func (bootstrapApache) Apply(context.Context, string, []byte) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func (bootstrapApache) ApplyVHost(context.Context, string, []byte, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (bootstrapApache) SetVHostEnabled(context.Context, string, string, bool) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (bootstrapApache) RemoveVHost(context.Context, string, string) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}
func (bootstrapApache) ValidateAndReload(context.Context) error { return nil }

type failingBootstrapApache struct{ bootstrapApache }

func (failingBootstrapApache) ValidateAndReload(context.Context) error {
	return errors.New("Apache configtest failed")
}

type bootstrapCommander struct{}

func (bootstrapCommander) Run(context.Context, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}
func (bootstrapCommander) RunWithStdin(context.Context, io.Reader, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}

type bootstrapSystemd struct {
	active  map[string]bool
	started []string
}

func (*bootstrapSystemd) Reload(context.Context, string) error  { return nil }
func (*bootstrapSystemd) Restart(context.Context, string) error { return nil }
func (systemd *bootstrapSystemd) Start(_ context.Context, unit string) error {
	systemd.active[unit] = true
	systemd.started = append(systemd.started, unit)
	return nil
}
func (*bootstrapSystemd) Stop(context.Context, string) error { return nil }
func (systemd *bootstrapSystemd) IsActive(_ context.Context, unit string) (bool, error) {
	return systemd.active[unit], nil
}
func (*bootstrapSystemd) Enable(context.Context, string) error  { return nil }
func (*bootstrapSystemd) Disable(context.Context, string) error { return nil }

func TestBootstrap_PrepareReturnsNoStepsWhenReady(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	operation, err := bootstrapService(fs, cfg).Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(operation.Steps) != 0 {
		t.Errorf("steps = %#v, want no-op plan", operation.Steps)
	}
}

func TestBootstrap_PrepareAddsMissingSystemDirectories(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	for _, path := range []string{meta.ConfigDir, meta.StateDir, cfg.Paths.ACMEChallenge, filepath.Join(cfg.Paths.ACMEChallenge, ".well-known", "acme-challenge"), meta.LogDir, cfg.Paths.VHosts} {
		delete(fs.entries, path)
	}
	operation, err := bootstrapService(fs, cfg).Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if got, want := len(operation.Steps), 7; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}
	for index, want := range []string{"create configuration directory", "create state directory", "create ACME challenge directory", "create ACME challenge content directory", "create log directory", "create vhosts root", "validate and reload Apache"} {
		if got := operation.Steps[index].Name; got != want {
			t.Errorf("step %d = %q, want %q", index, got, want)
		}
	}
}

func TestBootstrap_PrepareStartsInactiveRequiredServices(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	cfg.MariaDB.Enabled = true
	systemd := &bootstrapSystemd{active: map[string]bool{}}
	service := bootstrapService(fs, cfg)
	service.Systemd = systemd

	operation, err := service.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if got, want := len(operation.Steps), 3; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}
	for _, step := range operation.Steps {
		if err := step.Do(context.Background()); err != nil {
			t.Fatalf("%s: %v", step.Name, err)
		}
	}
	if diff := cmp.Diff([]string{"apache2", "mariadb.service"}, systemd.started); diff != "" {
		t.Errorf("started services mismatch (-want +got):\n%s", diff)
	}
}

func TestBootstrap_PrepareWithSkipOmitsOptionalArtifact(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	delete(fs.entries, meta.DeployHook)
	operation, err := bootstrapService(fs, cfg).PrepareWithSkip(context.Background(), []string{"deploy-hook"})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range operation.Steps {
		if step.Name == "install Certbot deploy hook" {
			t.Fatal("deploy hook step was not skipped")
		}
	}
}

func TestBootstrap_PrepareWithSkipRejectsUnknownArtifact(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	_, err := bootstrapService(fs, cfg).PrepareWithSkip(context.Background(), []string{"directories"})
	if err == nil {
		t.Fatal("PrepareWithSkip() error = nil, want unsupported skip")
	}
}

func TestBootstrap_PrepareRefusesExistingDirectoryWithWrongPermissions(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	entry := fs.entries[meta.LogDir]
	entry.mode = 0o755
	fs.entries[meta.LogDir] = entry
	_, err := bootstrapService(fs, cfg).Prepare(context.Background())
	if err == nil {
		t.Fatal("Prepare() error = nil, want permission refusal")
	}
}

func TestBootstrap_PrepareMigratesLegacyLogDirectoryMode(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	entry := fs.entries[meta.LogDir]
	entry.mode = 0o750
	fs.entries[meta.LogDir] = entry
	operation, err := bootstrapService(fs, cfg).Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if got, want := len(operation.Steps), 2; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}
	step := operation.Steps[0]
	if got, want := step.Name, "create log directory"; got != want {
		t.Errorf("step name = %q, want %q", got, want)
	}
	if err := step.Do(context.Background()); err != nil {
		t.Fatalf("migrate log directory: %v", err)
	}
	if got, want := fs.entries[meta.LogDir].mode, os.FileMode(0o751); got != want {
		t.Errorf("migrated mode = %04o, want %04o", got, want)
	}
	if err := step.Undo(context.Background()); err != nil {
		t.Fatalf("undo migration: %v", err)
	}
	if got, want := fs.entries[meta.LogDir].mode, os.FileMode(0o750); got != want {
		t.Errorf("restored mode = %04o, want %04o", got, want)
	}
}

func TestBootstrap_RunRollsBackCreatedDirectoryWhenApacheValidationFails(t *testing.T) {
	fs, cfg := readyBootstrapFS(t)
	delete(fs.entries, meta.ConfigDir)
	service := bootstrapService(fs, cfg)
	service.Apache = failingBootstrapApache{}
	service.Executor = plan.Executor{Journal: &subscriptionJournal{}, Locker: subscriptionLocker{}}
	_, _, err := service.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want Apache validation failure")
	}
	if _, exists := fs.entries[meta.ConfigDir]; exists {
		t.Error("created configuration directory remains after rollback")
	}
}

func bootstrapService(fs *bootstrapFS, cfg config.Config) BootstrapService {
	return BootstrapService{FS: fs, Modules: ApacheModules{FS: fs, AvailablePath: "/modules-available", EnabledPath: "/modules-enabled"}, Certificate: DefaultCertificate{FS: fs, Commands: bootstrapCommander{}, Directory: meta.DefaultSSLDir, Certificate: meta.DefaultSSLCertificate, Key: meta.DefaultSSLKey}, Apache: bootstrapApache{}, Config: cfg, AuditGroup: 4}
}

func readyBootstrapFS(t *testing.T) (*bootstrapFS, config.Config) {
	t.Helper()
	cfg := config.Config{Paths: config.Paths{VHosts: "/vhosts", ACMEChallenge: "/state/acme"}, Apache: config.Apache{Service: "apache2", SitesAvailable: "/sites-available", SitesEnabled: "/sites-enabled"}}
	fs := &bootstrapFS{entries: map[string]bootstrapEntry{}, links: map[string]string{}}
	for path, mode := range map[string]os.FileMode{meta.ConfigDir: 0o755, meta.StateDir: 0o700, cfg.Paths.ACMEChallenge: 0o755, filepath.Join(cfg.Paths.ACMEChallenge, ".well-known", "acme-challenge"): 0o755, meta.LogDir: 0o751, cfg.Paths.VHosts: 0o755} {
		fs.entries[path] = bootstrapEntry{dir: true, mode: mode}
	}
	for _, module := range RequiredApacheModules {
		source := filepath.Join("/modules-available", module+".load")
		fs.entries[source] = bootstrapEntry{mode: 0o644}
		fs.links[filepath.Join("/modules-enabled", module+".load")] = source
	}
	fs.entries[meta.DefaultSSLCertificate] = bootstrapEntry{mode: 0o644}
	fs.entries[meta.DefaultSSLKey] = bootstrapEntry{mode: 0o600}
	vhost, err := render.RenderDefaultApacheVHost(render.DefaultApacheVHost{CertificateFile: meta.DefaultSSLCertificate, KeyFile: meta.DefaultSSLKey})
	if err != nil {
		t.Fatalf("RenderDefaultApacheVHost() error = %v", err)
	}
	vhostPath := filepath.Join(cfg.Apache.SitesAvailable, meta.FilePrefix+"000-default.conf")
	fs.entries[vhostPath] = bootstrapEntry{data: vhost, mode: 0o640}
	fs.links[filepath.Join(cfg.Apache.SitesEnabled, filepath.Base(vhostPath))] = vhostPath
	fs.entries[meta.DeployHook] = bootstrapEntry{data: []byte("#!/bin/sh\n# GENERATED BY PROVCTL — DO NOT EDIT\n/usr/bin/provctl ssl deploy-hook --lineage \"$RENEWED_LINEAGE\" >> /var/log/provctl/deploy-hook.log 2>&1\nexit 0\n"), mode: 0o750}
	fs.entries[meta.LogrotateConfig] = bootstrapEntry{data: []byte("/var/log/provctl/*.log /var/log/provctl/*.jsonl {\n    weekly\n    rotate 14\n    missingok\n    notifempty\n    compress\n    delaycompress\n    create 0640 root adm\n}\n"), mode: 0o644}
	fs.entries[meta.AuditLog] = bootstrapEntry{mode: 0o640}
	return fs, cfg
}
