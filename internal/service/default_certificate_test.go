package service

import (
	"context"
	"io"
	"os"
	"testing"

	"provctl/internal/system"
)

type certificateFS struct{ files map[string]bool }

func (fs *certificateFS) Stat(path string) (os.FileInfo, error) {
	if fs.files[path] {
		return apacheFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (*certificateFS) ReadFile(string) ([]byte, error)                   { return nil, os.ErrNotExist }
func (*certificateFS) WriteFileAtomic(string, []byte, os.FileMode) error { return nil }
func (fs *certificateFS) Remove(path string) error                       { delete(fs.files, path); return nil }
func (*certificateFS) RemoveAll(string) error                            { return nil }
func (*certificateFS) MkdirAll(string, os.FileMode) error                { return nil }
func (*certificateFS) Chown(string, int, int) error                      { return nil }
func (*certificateFS) Chmod(string, os.FileMode) error                   { return nil }
func (*certificateFS) Symlink(string, string) error                      { return nil }
func (*certificateFS) ReadDir(string) ([]os.DirEntry, error)             { return nil, nil }
func (*certificateFS) EvalSymlinks(path string) (string, error)          { return path, nil }

type certificateCommander struct {
	fs          *certificateFS
	certificate string
}

func (commander certificateCommander) Run(context.Context, string, ...string) (system.Result, error) {
	commander.fs.files[commander.certificate] = true
	return system.Result{}, nil
}
func (certificateCommander) RunWithStdin(context.Context, io.Reader, string, ...string) (system.Result, error) {
	return system.Result{}, nil
}

func TestDefaultCertificate_EnsureIsIdempotent(t *testing.T) {
	cert, key := defaultCertificatePaths("/state/default-ssl")
	fs := &certificateFS{files: map[string]bool{}}
	undo, changed, err := (DefaultCertificate{FS: fs, Commands: certificateCommander{fs: fs, certificate: cert}, Directory: "/state/default-ssl", Certificate: cert, Key: key}).Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}
	fs.files[key] = true
	if err := undo(context.Background()); err != nil {
		t.Fatalf("undo() error = %v", err)
	}
	if len(fs.files) != 0 {
		t.Errorf("certificate files remain: %#v", fs.files)
	}
}
