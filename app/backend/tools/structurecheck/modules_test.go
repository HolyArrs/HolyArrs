package structurecheck

import (
	"flag"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var seedSQL = flag.Bool("seed", false, "rewrite testdata/sql-outside-store.txt from the files that break the SQL rules today")

const sqlAllowlistFile = "sql-outside-store.txt"

var (
	reSQL      = regexp.MustCompile(`(?is)\b(select\b.+?\bfrom\b|insert\s+(or\s+\w+\s+)?into\b|update\s+\w+\s+set\b|delete\s+from\b)`)
	reTable    = regexp.MustCompile(`(?i)\b(?:from|join|into|update)\s+([a-z_][a-z0-9_]*)`)
	internalAt = "app/backend/internal/"
)

// homeOf says which module or core package a file under internal/ belongs
// to now or will belong to after its move. "" means it is not judged.
func homeOf(rel string) string {
	tail, ok := strings.CutPrefix(rel, internalAt)
	if !ok {
		return ""
	}
	pkg, rest, _ := strings.Cut(tail, "/")
	if pkg == "core" {
		sub, _, _ := strings.Cut(rest, "/")
		return "core/" + sub
	}
	if _, ok := layers[pkg]; ok {
		return pkg
	}
	if h, ok := packageHome[pkg]; ok && h != "-" {
		return h
	}
	return ""
}

func isCoreHome(h string) bool { return strings.HasPrefix(h, "core/") }

// layerOf is the layer of a module home; core homes and unjudged homes
// report ok=false.
func layerOf(h string) (int, bool) {
	l, ok := layers[h]
	return l, ok
}

// isStoreFile reports whether SQL may live in the file: store*.go in a
// module, and anything in the database package (and the store package that
// becomes it).
func isStoreFile(rel string) bool {
	if strings.HasPrefix(rel, internalAt+"core/database/") || strings.HasPrefix(rel, internalAt+"store/") {
		return true
	}
	return strings.HasPrefix(filepath.Base(rel), "store")
}

// sqlLeftover is true for the leftover packages that are judged by a core home
// for imports but still hold other owners' tables: store (until t06b splits
// it) and parts (until t13 splits it).
func sqlLeftover(rel string) bool {
	return strings.HasPrefix(rel, internalAt+"store/") || strings.HasPrefix(rel, internalAt+"core/parts/")
}

// sqlFile is what the scan finds in one Go file.
type sqlFile struct {
	hasSQL bool
	tables []string
}

// scanSQL reads every non-test Go file under app/backend and reports the
// ones with SQL in a string literal and the known tables they name.
func scanSQL(t *testing.T, root string) map[string]sqlFile {
	t.Helper()
	out := map[string]sqlFile{}
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
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file := fset.AddFile(p, fset.Base(), len(src))
		var s scanner.Scanner
		s.Init(file, src, nil, 0)
		var f sqlFile
		seen := map[string]bool{}
		var lits []string
		for {
			_, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok != token.STRING {
				continue
			}
			v, err := strconv.Unquote(lit)
			if err != nil {
				continue
			}
			lits = append(lits, v)
			if reSQL.MatchString(v) {
				f.hasSQL = true
			}
		}
		if f.hasSQL {
			for _, v := range lits {
				for _, m := range reTable.FindAllStringSubmatch(v, -1) {
					name := strings.ToLower(m[1])
					if _, known := tableOwner[name]; known && !seen[name] {
						seen[name] = true
						f.tables = append(f.tables, name)
					}
				}
			}
			sort.Strings(f.tables)
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = f
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// foreignTables lists the tables a file names that its own package does not
// own. Files outside internal/ and the migrate orchestrator are not judged.
func foreignTables(rel string, f sqlFile) []string {
	home := homeOf(rel)
	if home == "" || home == "migrate" || sqlLeftover(rel) {
		return nil
	}
	var out []string
	for _, tb := range f.tables {
		if tableOwner[tb] != home {
			out = append(out, tb)
		}
	}
	return out
}

func loadSQLAllowlist(t *testing.T) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", sqlAllowlistFile))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}
		}
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out[l] = true
		}
	}
	return out
}

// TestSeedSQLAllowlist writes the allowlist when run with -seed. This file has
// the structure build tag, so run: go test -tags structure -run TestSeedSQLAllowlist ./tools/structurecheck/ -args -seed
func TestSeedSQLAllowlist(t *testing.T) {
	if !*seedSQL {
		t.Skip("run with -seed to rewrite the allowlist")
	}
	root := repoRoot(t)
	var names []string
	for rel, f := range scanSQL(t, root) {
		if !isStoreFile(rel) || len(foreignTables(rel, f)) > 0 {
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	if err := os.WriteFile(filepath.Join("testdata", sqlAllowlistFile), []byte(strings.Join(names, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSQLOnlyInStore: SQL lives in store files only. The allowlist holds the
// files that have not moved yet; it only shrinks.
func TestSQLOnlyInStore(t *testing.T) {
	root := repoRoot(t)
	allow := loadSQLAllowlist(t)
	for rel := range scanSQL(t, root) {
		if !isStoreFile(rel) && !allow[rel] {
			t.Errorf("%s: SQL outside a store file", rel)
		}
	}
}

// TestTableOwnership: a table is named in SQL only by the package that owns
// it. migrate is exempt as the orchestrator of its own multi-table writes.
func TestTableOwnership(t *testing.T) {
	root := repoRoot(t)
	allow := loadSQLAllowlist(t)
	for rel, f := range scanSQL(t, root) {
		if allow[rel] {
			continue
		}
		if bad := foreignTables(rel, f); len(bad) > 0 {
			t.Errorf("%s: names tables of other owners: %s", rel, strings.Join(bad, ", "))
		}
	}
}

// TestSQLAllowlistShrinks: an allowlisted file that no longer breaks a rule
// is removed from the list.
func TestSQLAllowlistShrinks(t *testing.T) {
	root := repoRoot(t)
	found := scanSQL(t, root)
	for rel := range loadSQLAllowlist(t) {
		f, ok := found[rel]
		if !ok || (isStoreFile(rel) && len(foreignTables(rel, f)) == 0) {
			t.Errorf("%s: no longer breaks the SQL rules, remove it from testdata/%s", rel, sqlAllowlistFile)
		}
	}
}

// TestModuleEdges: a module imports only modules on a lower layer.
// Packages that have not moved yet are judged by their future home.
func TestModuleEdges(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)
	for _, p := range goPackages(t, root) {
		rel, ok := strings.CutPrefix(p.ImportPath, mod+"/")
		if !ok {
			continue
		}
		from := homeOf(rel + "/x.go")
		fl, judged := layerOf(from)
		if !judged {
			continue
		}
		for _, imp := range p.Imports {
			irel, ok := strings.CutPrefix(imp, mod+"/")
			if !ok {
				continue
			}
			to := homeOf(irel + "/x.go")
			if to == "" || to == from || isCoreHome(to) {
				continue
			}
			tl, judgedTo := layerOf(to)
			if judgedTo && tl >= fl {
				t.Errorf("%s (%s, layer %d) imports %s (%s, layer %d)", rel, from, fl, irel, to, tl)
			}
		}
	}
}

// TestPackageMap: every package under internal/ is a module, a core package
// or listed in packageHome. Core is never a default home.
func TestPackageMap(t *testing.T) {
	root := repoRoot(t)
	for _, d := range subDirs(t, filepath.Join(root, "app/backend/internal")) {
		if d == "core" {
			continue
		}
		if _, ok := layers[d]; ok {
			continue
		}
		if _, ok := packageHome[d]; !ok {
			t.Errorf("internal/%s has no home in packageHome", d)
		}
	}
	for m := range layers {
		if !slices.Contains(Modules, m) {
			t.Errorf("layer table names %s, which is not a module", m)
		}
	}
	for _, m := range Modules {
		if _, ok := layers[m]; !ok {
			t.Errorf("module %s has no layer", m)
		}
	}
}

// TestCoreRowOrder: a core package imports only core packages on its own row
// or a lower one.
func TestCoreRowOrder(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)
	for _, p := range corePackages(t, root) {
		fromName := coreName(mod, p.ImportPath)
		fromRow, ok := coreRows[fromName]
		if !ok {
			t.Errorf("%s has no row in coreRows", p.ImportPath)
			continue
		}
		for _, imp := range slices.Concat(p.Imports, p.TestImports, p.XTestImports) {
			if !strings.HasPrefix(imp, mod+"/"+internalAt+"core/") {
				continue
			}
			toName := coreName(mod, imp)
			if toName == fromName {
				continue
			}
			if toRow, ok := coreRows[toName]; ok && toRow > fromRow {
				t.Errorf("%s (row %d) imports %s (row %d)", p.ImportPath, fromRow, imp, toRow)
			}
		}
	}
}

// coreName is the core package an import path sits in, sub-packages included.
func coreName(mod, importPath string) string {
	rest := strings.TrimPrefix(importPath, mod+"/"+internalAt+"core/")
	name, _, _ := strings.Cut(rest, "/")
	return name
}

// TestCoreNamesNoModuleTable: a core file never names a table a module owns,
// and the allowlist cannot excuse it. Core reaches module data through hooks
// that the module registers from wiring.go.
func TestCoreNamesNoModuleTable(t *testing.T) {
	root := repoRoot(t)
	for rel, f := range scanSQL(t, root) {
		if !isCoreHome(homeOf(rel)) || sqlLeftover(rel) {
			continue
		}
		for _, tb := range f.tables {
			if !isCoreHome(tableOwner[tb]) {
				t.Errorf("%s: core names the module table %s (owner %s)", rel, tb, tableOwner[tb])
			}
		}
	}
}
