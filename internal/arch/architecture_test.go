package arch

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchitecture_DomainHasNoProjectImports(t *testing.T) {
	assertNoImportsWithPrefix(t, "../domain", "provctl/")
}

func TestArchitecture_FrontendsDoNotAccessInfrastructure(t *testing.T) {
	for _, directory := range []string{"../cli", "../../tui"} {
		assertNoImportsWithPrefix(t, directory, "provctl/internal/system")
		assertNoImportsWithPrefix(t, directory, "provctl/internal/repository")
	}
}

func assertNoImportsWithPrefix(t *testing.T, relativeDirectory, forbidden string) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	directory := filepath.Clean(filepath.Join(filepath.Dir(source), relativeDirectory))
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatalf("list Go files in %s: %v", directory, err)
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if strings.HasPrefix(path, forbidden) {
				t.Errorf("%s imports forbidden package %q", file, path)
			}
		}
	}
}
