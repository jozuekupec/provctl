package arch

import (
	"go/parser"
	"go/token"
	"os"
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

func TestArchitecture_NoShellOrPinnedPHPVersion(t *testing.T) {
	for _, file := range goFiles(t, "..") {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(contents)
		if strings.Contains(text, `"sh", "-c"`) || strings.Contains(text, `"bash", "-c"`) {
			t.Errorf("%s invokes a shell", file)
		}
		if strings.Contains(text, "8.4") || strings.Contains(text, "8.5") {
			t.Errorf("%s pins a PHP version", file)
		}
	}
}

func assertNoImportsWithPrefix(t *testing.T, relativeDirectory, forbidden string) {
	t.Helper()
	for _, file := range goFiles(t, relativeDirectory) {
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

func goFiles(t *testing.T, relativeDirectory string) []string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	directory := filepath.Clean(filepath.Join(filepath.Dir(source), relativeDirectory))
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return nil
	}
	var files []string
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("list Go files in %s: %v", directory, err)
	}
	return files
}
