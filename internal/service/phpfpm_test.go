package service

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"time"

	"provctl/internal/system"
)

type phpFS struct{}

func (phpFS) Stat(path string) (os.FileInfo, error) {
	if path == "/etc/php/7.9/fpm/pool.d" || path == "/usr/sbin/php-fpm7.9" {
		return phpInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (phpFS) ReadDir(string) ([]os.DirEntry, error) {
	return []os.DirEntry{phpEntry{name: "7.9", directory: true}, phpEntry{name: "ignored"}}, nil
}
func (phpFS) ReadFile(string) ([]byte, error)                   { return nil, os.ErrNotExist }
func (phpFS) WriteFileAtomic(string, []byte, os.FileMode) error { return nil }
func (phpFS) Remove(string) error                               { return nil }
func (phpFS) RemoveAll(string) error                            { return nil }
func (phpFS) MkdirAll(string, os.FileMode) error                { return nil }
func (phpFS) Chown(string, int, int) error                      { return nil }
func (phpFS) Chmod(string, os.FileMode) error                   { return nil }
func (phpFS) Symlink(string, string) error                      { return nil }
func (phpFS) EvalSymlinks(path string) (string, error)          { return path, nil }

type phpEntry struct {
	name      string
	directory bool
}

func (entry phpEntry) Name() string         { return entry.name }
func (entry phpEntry) IsDir() bool          { return entry.directory }
func (phpEntry) Type() fs.FileMode          { return 0 }
func (phpEntry) Info() (fs.FileInfo, error) { return phpInfo{}, nil }

type phpInfo struct{}

func (phpInfo) Name() string       { return "php" }
func (phpInfo) Size() int64        { return 0 }
func (phpInfo) Mode() os.FileMode  { return os.ModeDir }
func (phpInfo) ModTime() time.Time { return time.Time{} }
func (phpInfo) IsDir() bool        { return true }
func (phpInfo) Sys() any           { return nil }

type phpSystemd struct{}

func (phpSystemd) Reload(context.Context, string) error           { return nil }
func (phpSystemd) Restart(context.Context, string) error          { return nil }
func (phpSystemd) Start(context.Context, string) error            { return nil }
func (phpSystemd) Stop(context.Context, string) error             { return nil }
func (phpSystemd) IsActive(context.Context, string) (bool, error) { return true, nil }
func (phpSystemd) Enable(context.Context, string) error           { return nil }
func (phpSystemd) Disable(context.Context, string) error          { return nil }

func TestSelectPHPFPM_UsesHighestVersionWhenUnconfigured(t *testing.T) {
	available := []PHPFPMVersion{{Version: "7.9"}, {Version: "10.2"}}
	got, err := SelectPHPFPM("", available)
	if err != nil {
		t.Fatalf("SelectPHPFPM() error = %v", err)
	}
	if got.Version != "10.2" {
		t.Errorf("selected %q, want highest version", got.Version)
	}
}

func TestDiscoverPHPFPM_RequiresPoolDirectoryAndBinary(t *testing.T) {
	versions, err := DiscoverPHPFPM(context.Background(), phpFS{}, phpSystemd{})
	if err != nil {
		t.Fatalf("DiscoverPHPFPM() error = %v", err)
	}
	if got, want := len(versions), 1; got != want {
		t.Fatalf("versions = %d, want %d", got, want)
	}
	if versions[0].Version != "7.9" || !versions[0].Active {
		t.Errorf("version = %#v", versions[0])
	}
}

var _ system.FS = phpFS{}
var _ system.Systemd = phpSystemd{}

func TestSelectPHPFPM_RejectsMissingConfiguredVersion(t *testing.T) {
	_, err := SelectPHPFPM("9.9", []PHPFPMVersion{{Version: "7.9"}})
	if err == nil {
		t.Fatal("SelectPHPFPM() error = nil, want failure")
	}
}
