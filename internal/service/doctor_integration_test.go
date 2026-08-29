package service

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"provctl/internal/config"
	"provctl/internal/system"
)

type doctorIdentity int

func (identity doctorIdentity) EUID() int { return int(identity) }

type doctorFS struct{}

func (doctorFS) Stat(string) (os.FileInfo, error)                  { return doctorFileInfo{}, nil }
func (doctorFS) ReadFile(string) ([]byte, error)                   { return nil, os.ErrNotExist }
func (doctorFS) WriteFileAtomic(string, []byte, os.FileMode) error { return nil }
func (doctorFS) Remove(string) error                               { return nil }
func (doctorFS) MkdirAll(string, os.FileMode) error                { return nil }
func (doctorFS) Chown(string, int, int) error                      { return nil }
func (doctorFS) Chmod(string, os.FileMode) error                   { return nil }
func (doctorFS) Symlink(string, string) error                      { return nil }
func (doctorFS) ReadDir(string) ([]os.DirEntry, error)             { return nil, nil }
func (doctorFS) EvalSymlinks(path string) (string, error)          { return path, nil }

type doctorFileInfo struct{}

func (doctorFileInfo) Name() string       { return "directory" }
func (doctorFileInfo) Size() int64        { return 0 }
func (doctorFileInfo) Mode() os.FileMode  { return os.ModeDir | 0o700 }
func (doctorFileInfo) ModTime() time.Time { return time.Time{} }
func (doctorFileInfo) IsDir() bool        { return true }
func (doctorFileInfo) Sys() any           { return nil }

type doctorCommander struct{ cron string }

func (commander doctorCommander) Run(_ context.Context, name string, args ...string) (system.Result, error) {
	if name == "/usr/sbin/apachectl" && len(args) == 1 && args[0] == "-M" {
		return system.Result{Stdout: "proxy_module proxy_fcgi_module proxy_http_module ssl_module rewrite_module headers_module"}, nil
	}
	if name == "/usr/bin/crontab" {
		return system.Result{Stdout: commander.cron}, nil
	}
	return system.Result{Stdout: "version"}, nil
}
func (doctorCommander) RunWithStdin(context.Context, io.Reader, string, ...string) (system.Result, error) {
	return system.Result{Stdout: "1\n"}, nil
}

type doctorSystemd struct{ timer bool }

func (doctorSystemd) Reload(context.Context, string) error  { return nil }
func (doctorSystemd) Restart(context.Context, string) error { return nil }
func (doctorSystemd) Start(context.Context, string) error   { return nil }
func (doctorSystemd) Stop(context.Context, string) error    { return nil }
func (state doctorSystemd) IsActive(_ context.Context, unit string) (bool, error) {
	return unit == "apache2" || unit == "certbot.timer" && state.timer, nil
}
func (doctorSystemd) Enable(context.Context, string) error  { return nil }
func (doctorSystemd) Disable(context.Context, string) error { return nil }

func TestDoctor_RenewalConflictIsWarning(t *testing.T) {
	cfg := config.Config{Meta: config.Meta{ConfigVersion: config.CurrentVersion}, Paths: config.Paths{VHosts: "/vhosts"}, Apache: config.Apache{Service: "apache2"}, MariaDB: config.MariaDB{Enabled: true}}
	doctor := NewDoctor(doctorFS{}, doctorCommander{cron: "certbot renew"}, doctorSystemd{timer: true}, doctorIdentity(0))
	for _, check := range doctor.Run(context.Background(), cfg) {
		if check.Name == "certificate renewal" && check.Status == CheckWarn {
			return
		}
	}
	t.Fatal("certificate renewal warning was not reported")
}

var _ system.FS = doctorFS{}
var _ system.Commander = doctorCommander{}
var _ system.Systemd = doctorSystemd{}
