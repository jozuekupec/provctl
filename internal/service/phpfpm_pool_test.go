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

type poolCommander struct{ calls, failOn int }

func (commander *poolCommander) Run(context.Context, string, ...string) (system.Result, error) {
	commander.calls++
	if commander.calls == commander.failOn {
		return system.Result{Stderr: "invalid pool"}, errors.New("exit status 1")
	}
	return system.Result{}, nil
}
func (*poolCommander) RunWithStdin(context.Context, io.Reader, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}

type poolFS struct {
	files  map[string][]byte
	socket bool
}

func (fs *poolFS) Stat(path string) (os.FileInfo, error) {
	if path == "/run/php/provctl-acme.sock" && fs.socket {
		return poolFileInfo{mode: os.ModeSocket | 0o660}, nil
	}
	if _, ok := fs.files[path]; ok {
		return poolFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (fs *poolFS) ReadFile(path string) ([]byte, error) { return fs.files[path], nil }
func (fs *poolFS) WriteFileAtomic(path string, data []byte, _ os.FileMode) error {
	fs.files[path] = append([]byte(nil), data...)
	return nil
}
func (fs *poolFS) Remove(path string) error              { delete(fs.files, path); return nil }
func (*poolFS) RemoveAll(string) error                   { return nil }
func (*poolFS) MkdirAll(string, os.FileMode) error       { return nil }
func (*poolFS) Chown(string, int, int) error             { return nil }
func (*poolFS) Chmod(string, os.FileMode) error          { return nil }
func (*poolFS) Symlink(string, string) error             { return nil }
func (*poolFS) ReadDir(string) ([]os.DirEntry, error)    { return nil, nil }
func (*poolFS) EvalSymlinks(path string) (string, error) { return path, nil }

type poolFileInfo struct{ mode os.FileMode }

func (poolFileInfo) Name() string               { return "pool" }
func (poolFileInfo) Size() int64                { return 0 }
func (info poolFileInfo) Mode() os.FileMode     { return info.mode }
func (poolFileInfo) ModTime() (value time.Time) { return }
func (poolFileInfo) IsDir() bool                { return false }
func (poolFileInfo) Sys() any                   { return nil }

func TestPHPFPM_ApplyPoolRestoresPreviousPoolOnConfigFailure(t *testing.T) {
	fs := &poolFS{files: map[string][]byte{"/etc/php/version/fpm/pool.d/provctl-acme.conf": []byte("old")}}
	commands, systemd := &poolCommander{failOn: 1}, &apacheSystemd{active: true}
	_, err := (PHPFPM{FS: fs, Commands: commands, Systemd: systemd}).ApplyPool(context.Background(), PHPFPMVersion{Binary: "/usr/sbin/php-fpm7.9", Service: "php7.9-fpm.service"}, "/etc/php/version/fpm/pool.d/provctl-acme.conf", []byte("new"), "/run/php/provctl-acme.sock")
	if err == nil {
		t.Fatal("ApplyPool() error = nil, want failure")
	}
	if got := string(fs.files["/etc/php/version/fpm/pool.d/provctl-acme.conf"]); got != "old" {
		t.Errorf("restored contents = %q, want old", got)
	}
	if got, want := commands.calls, 2; got != want {
		t.Errorf("configtest calls = %d, want %d", got, want)
	}
	if got, want := systemd.reloads, 1; got != want {
		t.Errorf("reloads = %d, want %d", got, want)
	}
}

func TestPHPFPM_ApplyPoolVerifiesSocket(t *testing.T) {
	fs := &poolFS{files: map[string][]byte{}, socket: true}
	undo, err := (PHPFPM{FS: fs, Commands: &poolCommander{}, Systemd: &apacheSystemd{active: true}}).ApplyPool(context.Background(), PHPFPMVersion{Binary: "/usr/sbin/php-fpm7.9", Service: "php7.9-fpm.service"}, "/etc/php/version/fpm/pool.d/provctl-acme.conf", []byte("new"), "/run/php/provctl-acme.sock")
	if err != nil {
		t.Fatalf("ApplyPool() error = %v", err)
	}
	if err := undo(context.Background()); err != nil {
		t.Fatalf("undo() error = %v", err)
	}
	if _, exists := fs.files["/etc/php/version/fpm/pool.d/provctl-acme.conf"]; exists {
		t.Error("new pool remains after undo")
	}
}

func TestPHPFPM_RemovePoolRestoresPreviousPool(t *testing.T) {
	path := "/etc/php/version/fpm/pool.d/provctl-acme.conf"
	fs := &poolFS{files: map[string][]byte{path: []byte("old")}}
	undo, err := (PHPFPM{FS: fs, Commands: &poolCommander{}, Systemd: &apacheSystemd{active: true}}).RemovePool(context.Background(), PHPFPMVersion{Binary: "/usr/sbin/php-fpm7.9", Service: "php7.9-fpm.service"}, path)
	if err != nil {
		t.Fatalf("RemovePool() error = %v", err)
	}
	if _, exists := fs.files[path]; exists {
		t.Error("pool remains after RemovePool()")
	}
	if err := undo(context.Background()); err != nil {
		t.Fatalf("undo() error = %v", err)
	}
	if got := string(fs.files[path]); got != "old" {
		t.Errorf("restored contents = %q, want old", got)
	}
}
