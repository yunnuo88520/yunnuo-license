package licensing

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBusinessCoreCannotEscapeScopedRepositories(t *testing.T) {
	entries, e := os.ReadDir(".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, e := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, imp := range file.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if path == "database/sql" || strings.Contains(path, "/internal/store") || strings.Contains(path, "sqlite") || strings.Contains(path, "mysql") || strings.Contains(path, "gorm") {
				t.Errorf("business core %s imports SQL/storage escape hatch %s", entry.Name(), path)
			}
		}
		if entry.Name() == "access.go" || entry.Name() == "model.go" || entry.Name() == "keys.go" {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Name == "BindScope" {
					t.Errorf("%s invents application authority outside the access boundary", filepath.Base(entry.Name()))
				}
			case *ast.SelectorExpr:
				if fun.Sel.Name == "BindScope" {
					t.Errorf("%s bypasses the access boundary", entry.Name())
				}
			}
			return true
		})
	}
}
