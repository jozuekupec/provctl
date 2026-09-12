// Package fsbrowse provides sorted directory entries for terminal path pickers.
package fsbrowse

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Mode controls which entries are eligible for selection.
type Mode int

const (
	Dirs Mode = iota
	All
)

// Entry is one filesystem item.
type Entry struct {
	Name string
	Dir  bool
}

// List returns directories first, then files; both groups are case-insensitively sorted.
func List(dir string, mode Mode) ([]Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		isDir := item.IsDir()
		if !isDir && mode == Dirs {
			continue
		}
		entries = append(entries, Entry{Name: item.Name(), Dir: isDir})
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].Dir != entries[right].Dir {
			return entries[left].Dir
		}
		return strings.ToLower(entries[left].Name) < strings.ToLower(entries[right].Name)
	})
	return entries, nil
}

// Parent returns the directory immediately above dir, or false at its root.
func Parent(dir string) (string, bool) {
	clean := filepath.Clean(dir)
	parent := filepath.Dir(clean)
	return parent, parent != clean
}

// NearestExisting returns the closest existing directory at or above path.
func NearestExisting(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	current := filepath.Clean(path)
	for {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			return current
		}
		parent, ok := Parent(current)
		if !ok {
			return ""
		}
		current = parent
	}
}

// Browse resolves a possibly unfinished path to its nearest existing directory
// and lists that directory. It is intended to run in a Tea command, not Update.
func Browse(path string, mode Mode) (string, []Entry, error) {
	dir := NearestExisting(path)
	if dir == "" {
		dir = string(filepath.Separator)
	}
	entries, err := List(dir, mode)
	return dir, entries, err
}
