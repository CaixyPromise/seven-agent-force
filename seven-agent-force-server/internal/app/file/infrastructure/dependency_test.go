package infrastructure

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestInfrastructureDoesNotImportUpperFileLayers keeps the repository adapter
// behind the file module's application and facade contracts. It deliberately
// excludes tests because integration fixtures compose real layers by design.
func TestInfrastructureDoesNotImportUpperFileLayers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read infrastructure directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Clean(entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			if isUpperFileImport(importPath) {
				t.Errorf("%s imports upper file layer %q", path, importPath)
			}
		}
	}
}

func isUpperFileImport(importPath string) bool {
	for _, layer := range []string{"facade", "application", "handler", "controller", "job", "listener"} {
		if strings.Contains(importPath, "/internal/app/file/"+layer) {
			return true
		}
	}
	return false
}
