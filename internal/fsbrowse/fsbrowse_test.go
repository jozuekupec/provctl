package fsbrowse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestList_SortsDirectoriesBeforeFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"zeta", "alpha"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Zebra.txt", "apple.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := List(root, All)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Name
	}
	if want := "alpha,zeta,apple.txt,Zebra.txt"; strings.Join(got, ",") != want {
		t.Fatalf("entries = %v, want %s", got, want)
	}
}

func TestNearestExisting_ClimbsToExistingDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := NearestExisting(filepath.Join(child, "missing", "leaf")); got != child {
		t.Fatalf("nearest existing = %q, want %q", got, child)
	}
}
