package structurecheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// literalSets reads every non-test Go file under app/backend and returns the
// HTTP header names written as literals in Header calls (Get, Set, Add, Del,
// Values on something called Header) and the JSON names in struct tags. A
// rename pass renames identifiers, and it must never change these.
func literalSets(t *testing.T, root string) (headers, jsonKeys []string) {
	t.Helper()
	hs, js := map[string]bool{}, map[string]bool{}
	base := filepath.Join(root, "app/backend")
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != base && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			if filepath.ToSlash(p) == filepath.ToSlash(filepath.Join(base, "tools/structurecheck")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok || len(x.Args) == 0 || !isHeaderOp(sel.Sel.Name) || !endsInHeader(sel.X) {
					return true
				}
				if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						hs[v] = true
					}
				}
			case *ast.Field:
				if x.Tag == nil {
					return true
				}
				tag, err := strconv.Unquote(x.Tag.Value)
				if err != nil {
					return true
				}
				if name, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ","); name != "" && name != "-" {
					js[name] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for k := range hs {
		headers = append(headers, k)
	}
	for k := range js {
		jsonKeys = append(jsonKeys, k)
	}
	sort.Strings(headers)
	sort.Strings(jsonKeys)
	return headers, jsonKeys
}

func isHeaderOp(name string) bool {
	switch name {
	case "Get", "Set", "Add", "Del", "Values":
		return true
	}
	return false
}

// endsInHeader is true for r.Header and w.Header().
func endsInHeader(e ast.Expr) bool {
	if c, ok := e.(*ast.CallExpr); ok {
		e = c.Fun
	}
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Header"
}

// TestHeaderAndJSONLiteralsUnchanged: the HTTP header names and JSON keys
// the server writes and reads stay the same. A mechanical rename once turned
// Last-Event-ID into Last-LiveEvent-ID; this fails on any such change.
func TestHeaderAndJSONLiteralsUnchanged(t *testing.T) {
	headers, keys := literalSets(t, repoRoot(t))
	if len(headers) < 20 || len(keys) < 200 {
		t.Fatalf("found only %d header names and %d JSON keys", len(headers), len(keys))
	}
	compare(t, "header-literals.txt", headers)
	compare(t, "json-keys.txt", keys)
}
