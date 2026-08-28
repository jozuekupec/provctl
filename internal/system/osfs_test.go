package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFS_WriteFileAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.toml")
	if err := (OSFS{}).WriteFileAtomic(path, []byte("updated"), 0o640); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != "updated" {
		t.Errorf("contents = %q, want %q", contents, "updated")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("mode = %#o, want %#o", got, 0o640)
	}
}
