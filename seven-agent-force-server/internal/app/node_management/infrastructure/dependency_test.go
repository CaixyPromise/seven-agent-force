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

func TestInfrastructureDoesNotImportUpperNodeManagementLayers(t *testing.T) {
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
			if isUpperNodeManagementImport(importPath) {
				position := file.Pos()
				if spec.Pos().IsValid() {
					position = spec.Pos()
				}
				t.Errorf("%s:%d imports upper node_management layer %q", path, position, importPath)
			}
		}
	}
}

func isUpperNodeManagementImport(importPath string) bool {
	for _, layer := range []string{"facade", "application", "handler", "controller"} {
		if strings.Contains(importPath, "/internal/app/node_management/"+layer) {
			return true
		}
	}
	return false
}
