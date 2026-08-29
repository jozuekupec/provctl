package system

import (
	"fmt"
	"os"
	"path/filepath"
)

// OSFS is the production filesystem implementation.
type OSFS struct{}

func (OSFS) Stat(path string) (os.FileInfo, error)        { return os.Stat(path) }
func (OSFS) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (OSFS) Remove(path string) error                     { return os.Remove(path) }
func (OSFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (OSFS) MkdirAll(path string, mode os.FileMode) error { return os.MkdirAll(path, mode) }
func (OSFS) Chown(path string, uid, gid int) error        { return os.Chown(path, uid, gid) }
func (OSFS) Chmod(path string, mode os.FileMode) error    { return os.Chmod(path, mode) }
func (OSFS) Symlink(oldname, newname string) error        { return os.Symlink(oldname, newname) }
func (OSFS) ReadDir(path string) ([]os.DirEntry, error)   { return os.ReadDir(path) }
func (OSFS) EvalSymlinks(path string) (string, error)     { return filepath.EvalSymlinks(path) }

func (OSFS) WriteFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".provctl-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set mode on temporary file for %q: %w", path, err)
	}
	if _, err = temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file for %q: %w", path, err)
	}
	if err = temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary file for %q: %w", path, err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file for %q: %w", path, err)
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %q atomically: %w", path, err)
	}
	return nil
}
