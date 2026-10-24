package workforce

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Browser and machine APIs under Workforce share the Framework response
// envelope. Controllers must not bypass it with RequestContext.JSON, because
// doing so permits incompatible top-level code types and drops trace metadata.
func TestControllersUseFrameworkResponseEnvelope(t *testing.T) {
	t.Helper()
	var violations []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || !strings.Contains(filepath.ToSlash(path), "/controller/") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			// The Provider invocation endpoint is the one protocol boundary that
			// must return an OpenAI-compatible error object to the local Provider
			// proxy. It is not a Framework response envelope.
			if filepath.ToSlash(path) == "providerinvocation/controller/http.go" && function.Name.Name == "writeError" {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "JSON" {
					return true
				}
				position := fset.Position(call.Pos())
				violations = append(violations, position.String())
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("Workforce controllers must use internal/common/response so top-level code remains an integer; direct RequestContext.JSON calls: %s", strings.Join(violations, ", "))
	}
}
