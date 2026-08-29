package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type moduleFS struct{ files, links map[string]string }

func (fs *moduleFS) Stat(path string) (os.FileInfo, error) {
	if _, ok := fs.files[path]; ok {
		return apacheFileInfo{}, nil
	}
	if _, ok := fs.links[path]; ok {
		return apacheFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (*moduleFS) ReadFile(string) ([]byte, error)                   { return nil, os.ErrNotExist }
func (*moduleFS) WriteFileAtomic(string, []byte, os.FileMode) error { return nil }
func (fs *moduleFS) Remove(path string) error                       { delete(fs.links, path); return nil }
func (*moduleFS) RemoveAll(string) error                            { return nil }
func (*moduleFS) MkdirAll(string, os.FileMode) error                { return nil }
func (*moduleFS) Chown(string, int, int) error                      { return nil }
func (*moduleFS) Chmod(string, os.FileMode) error                   { return nil }
func (fs *moduleFS) Symlink(oldname, newname string) error          { fs.links[newname] = oldname; return nil }
func (*moduleFS) ReadDir(string) ([]os.DirEntry, error)             { return nil, nil }
func (fs *moduleFS) EvalSymlinks(path string) (string, error) {
	target, ok := fs.links[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return target, nil
}

func TestApacheModules_EnableRequiredCreatesAndUndoesLinks(t *testing.T) {
	fs := &moduleFS{files: map[string]string{}, links: map[string]string{}}
	for _, module := range RequiredApacheModules {
		fs.files[filepath.Join("/available", module+".load")] = ""
	}
	undo, changed, err := (ApacheModules{FS: fs, AvailablePath: "/available", EnabledPath: "/enabled"}).EnableRequired(context.Background())
	if err != nil {
		t.Fatalf("EnableRequired() error = %v", err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}
	if got, want := len(fs.links), len(RequiredApacheModules); got != want {
		t.Errorf("links = %d, want %d", got, want)
	}
	if err := undo(context.Background()); err != nil {
		t.Fatalf("undo() error = %v", err)
	}
	if len(fs.links) != 0 {
		t.Errorf("links remain after undo: %#v", fs.links)
	}
}

func TestApacheModules_EnableRequiredIsIdempotent(t *testing.T) {
	fs := &moduleFS{files: map[string]string{}, links: map[string]string{}}
	for _, module := range RequiredApacheModules {
		source := filepath.Join("/available", module+".load")
		fs.files[source] = ""
		fs.links[filepath.Join("/enabled", module+".load")] = source
	}
	_, changed, err := (ApacheModules{FS: fs, AvailablePath: "/available", EnabledPath: "/enabled"}).EnableRequired(context.Background())
	if err != nil {
		t.Fatalf("EnableRequired() error = %v", err)
	}
	if changed {
		t.Error("changed = true, want false")
	}
}
