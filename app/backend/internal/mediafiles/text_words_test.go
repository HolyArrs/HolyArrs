package mediafiles

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// User-facing text must say "Release", not the Go type name.
func TestUserTextHasNoTypeNames(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(node ast.Node) bool {
			if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "ReleaseFacts") {
				t.Errorf("%s: user text names ReleaseFacts: %s", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
	}
}
