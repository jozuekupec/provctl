package service

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"provctl/internal/system"
)

type apacheFS struct{ files map[string][]byte }

func (fs *apacheFS) Stat(path string) (os.FileInfo, error) {
	if _, exists := fs.files[path]; !exists {
		return nil, os.ErrNotExist
	}
	return apacheFileInfo{}, nil
}
func (fs *apacheFS) ReadFile(path string) ([]byte, error) { return fs.files[path], nil }
func (fs *apacheFS) WriteFileAtomic(path string, data []byte, _ os.FileMode) error {
	fs.files[path] = append([]byte(nil), data...)
	return nil
}
func (fs *apacheFS) Remove(path string) error                 { delete(fs.files, path); return nil }
func (fs *apacheFS) RemoveAll(string) error                   { return nil }
func (fs *apacheFS) MkdirAll(string, os.FileMode) error       { return nil }
func (fs *apacheFS) Chown(string, int, int) error             { return nil }
func (fs *apacheFS) Chmod(string, os.FileMode) error          { return nil }
func (fs *apacheFS) Symlink(string, string) error             { return nil }
func (fs *apacheFS) ReadDir(string) ([]os.DirEntry, error)    { return nil, nil }
func (fs *apacheFS) EvalSymlinks(path string) (string, error) { return path, nil }

type apacheFileInfo struct{}

func (apacheFileInfo) Name() string       { return "file" }
func (apacheFileInfo) Size() int64        { return 0 }
func (apacheFileInfo) Mode() os.FileMode  { return 0o640 }
func (apacheFileInfo) ModTime() time.Time { return time.Time{} }
func (apacheFileInfo) IsDir() bool        { return false }
func (apacheFileInfo) Sys() any           { return nil }

type apacheCommander struct {
	calls  int
	failOn int
}

func (commander *apacheCommander) Run(context.Context, string, ...string) (system.Result, error) {
	commander.calls++
	if commander.calls == commander.failOn {
		return system.Result{Stderr: "syntax error"}, errors.New("exit status 1")
	}
	return system.Result{}, nil
}
func (*apacheCommander) RunWithStdin(context.Context, io.Reader, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}

type apacheSystemd struct {
	reloads int
	active  bool
}

func (systemd *apacheSystemd) Reload(context.Context, string) error { systemd.reloads++; return nil }
func (*apacheSystemd) Restart(context.Context, string) error        { return nil }
func (*apacheSystemd) Start(context.Context, string) error          { return nil }
func (*apacheSystemd) Stop(context.Context, string) error           { return nil }
func (systemd *apacheSystemd) IsActive(context.Context, string) (bool, error) {
	return systemd.active, nil
}
func (*apacheSystemd) Enable(context.Context, string) error  { return nil }
func (*apacheSystemd) Disable(context.Context, string) error { return nil }

func TestApache_ApplyRestoresPreviousFileOnConfigFailure(t *testing.T) {
	fs := &apacheFS{files: map[string][]byte{"/etc/apache2/sites-available/provctl-acme.conf": []byte("old")}}
	commands, systemd := &apacheCommander{failOn: 2}, &apacheSystemd{active: true}
	_, err := (Apache{FS: fs, Commands: commands, Systemd: systemd, Service: "apache2"}).Apply(context.Background(), "/etc/apache2/sites-available/provctl-acme.conf", []byte("new"))
	if err == nil {
		t.Fatal("Apply() error = nil, want failure")
	}
	if got, want := string(fs.files["/etc/apache2/sites-available/provctl-acme.conf"]), "old"; got != want {
		t.Errorf("restored contents = %q, want %q", got, want)
	}
	if got, want := commands.calls, 3; got != want {
		t.Errorf("configtest calls = %d, want %d", got, want)
	}
	if systemd.reloads != 1 {
		t.Errorf("reloads = %d, want 1 after restore", systemd.reloads)
	}
}

func TestApache_ApplyReturnsRollbackForSuccessfulChange(t *testing.T) {
	fs := &apacheFS{files: map[string][]byte{}}
	commands, systemd := &apacheCommander{}, &apacheSystemd{active: true}
	undo, err := (Apache{FS: fs, Commands: commands, Systemd: systemd, Service: "apache2"}).Apply(context.Background(), "/etc/apache2/sites-available/provctl-acme.conf", []byte("new"))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := undo(context.Background()); err != nil {
		t.Fatalf("undo() error = %v", err)
	}
	if _, exists := fs.files["/etc/apache2/sites-available/provctl-acme.conf"]; exists {
		t.Error("new file remains after undo")
	}
}
