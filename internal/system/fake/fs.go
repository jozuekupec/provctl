package fake

import (
	"os"

	"provctl/internal/system"
)

// FS delegates filesystem operations to callbacks supplied by a test.
type FS struct {
	StatFunc         func(string) (os.FileInfo, error)
	ReadFileFunc     func(string) ([]byte, error)
	WriteFileFunc    func(string, []byte, os.FileMode) error
	RemoveFunc       func(string) error
	MkdirAllFunc     func(string, os.FileMode) error
	ChownFunc        func(string, int, int) error
	ChmodFunc        func(string, os.FileMode) error
	SymlinkFunc      func(string, string) error
	ReadDirFunc      func(string) ([]os.DirEntry, error)
	EvalSymlinksFunc func(string) (string, error)
}

var _ system.FS = (*FS)(nil)

func (f *FS) Stat(path string) (os.FileInfo, error) { return f.StatFunc(path) }
func (f *FS) ReadFile(path string) ([]byte, error)  { return f.ReadFileFunc(path) }
func (f *FS) WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	return f.WriteFileFunc(path, data, mode)
}
func (f *FS) Remove(path string) error                     { return f.RemoveFunc(path) }
func (f *FS) MkdirAll(path string, mode os.FileMode) error { return f.MkdirAllFunc(path, mode) }
func (f *FS) Chown(path string, uid, gid int) error        { return f.ChownFunc(path, uid, gid) }
func (f *FS) Chmod(path string, mode os.FileMode) error    { return f.ChmodFunc(path, mode) }
func (f *FS) Symlink(oldname, newname string) error        { return f.SymlinkFunc(oldname, newname) }
func (f *FS) ReadDir(path string) ([]os.DirEntry, error)   { return f.ReadDirFunc(path) }
func (f *FS) EvalSymlinks(path string) (string, error)     { return f.EvalSymlinksFunc(path) }
