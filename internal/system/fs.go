package system

import "os"

type FS interface {
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	WriteFileAtomic(path string, data []byte, mode os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	MkdirAll(path string, mode os.FileMode) error
	Chown(path string, uid, gid int) error
	Chmod(path string, mode os.FileMode) error
	Symlink(oldname, newname string) error
	ReadDir(path string) ([]os.DirEntry, error)
	EvalSymlinks(path string) (string, error)
}

// FileMover is an optional capability for operations that require an atomic
// rename within one filesystem, such as restoring staged data.
type FileMover interface {
	Rename(oldPath, newPath string) error
}
