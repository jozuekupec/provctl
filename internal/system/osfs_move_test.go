package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFS_RenameMovesWithinFilesystem(t *testing.T) {
	directory := t.TempDir()
	oldPath, newPath := filepath.Join(directory, "old"), filepath.Join(directory, "new")
	if err := os.WriteFile(oldPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (OSFS{}).Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path error = %v", err)
	}
	contents, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "data" {
		t.Errorf("contents = %q", contents)
	}
}
