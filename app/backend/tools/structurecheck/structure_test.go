package structurecheck

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Modules are the module packages allowed under internal/, besides core.
var Modules = []string{"suggest", "importer", "library", "requests", "movies", "shows", "music", "sources", "downloads", "subtitles", "discover",
	"mediafiles", "metadata", "mediaservers", "accounts", "alerts", "migrate", "arrapi", "ai", "backups",
	"health", "updates", "setup"}

// CoreParts are the packages allowed under internal/core/.
var CoreParts = []string{"database", "webserver", "outbound", "jobs", "logs", "files", "secrets", "startup", "releasename", "model", "parts"}

// sourceRoots are the top folders the guards walk.
var sourceRoots = []string{"app"}

// internalRoots are the folders holding module packages.
var internalRoots = []string{"app/backend/internal", "internal"}

type goPackage struct {
	ImportPath   string
	Dir          string
	GoFiles      []string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

func goPackages(t *testing.T, root string) []goPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []goPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p goPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

func modulePath(t *testing.T, root string) string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if m, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module "); ok {
			return strings.TrimSpace(m)
		}
	}
	t.Fatal("no module line in go.mod")
	return ""
}

// split turns an import path into its module name and the rest below it,
// or ok=false when the path is not under an internal root.
func split(mod, importPath string) (module, rest string, ok bool) {
	p, found := strings.CutPrefix(importPath, mod+"/")
	if !found {
		return "", "", false
	}
	for _, r := range internalRoots {
		if tail, found := strings.CutPrefix(p, r+"/"); found {
			module, rest, _ = strings.Cut(tail, "/")
			return module, rest, true
		}
	}
	return "", "", false
}

func subDirs(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// walkSource calls fn for every file under the source roots.
func walkSource(t *testing.T, root string, fn func(rel string, d fs.DirEntry)) {
	t.Helper()
	for _, sr := range sourceRoots {
		base := filepath.Join(root, sr)
		if !exists(root, sr) {
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != base && (skipDir(d.Name()) || d.Name() == "e2e" || d.Name() == "static") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			fn(filepath.ToSlash(rel), d)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTopLayout(t *testing.T) {
	root := repoRoot(t)
	for _, d := range []string{"app/backend/cmd/holyarrs", "app/backend/internal/core", "app/frontend/src", "app/tests", "deploy", "docs/start", "docs/dev"} {
		if !exists(root, d) {
			t.Errorf("%s is missing", d)
		}
	}
	for _, d := range []string{"cmd", "internal", "web", "services", "examples", "tools", "Dockerfile"} {
		if exists(root, d) {
			t.Errorf("%s should not be at the repo root", d)
		}
	}
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "app/frontend/dist/.gitkeep")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Errorf("app/frontend/dist/.gitkeep is not tracked")
	}
}

func TestModuleSet(t *testing.T) {
	root := repoRoot(t)
	if exists(root, "internal") {
		t.Errorf("root internal/ still exists")
	}
	for _, d := range subDirs(t, filepath.Join(root, "app/backend/internal")) {
		if d != "core" && !slices.Contains(Modules, d) {
			t.Errorf("app/backend/internal/%s is not a module", d)
		}
	}
}

func TestCoreSet(t *testing.T) {
	root := repoRoot(t)
	got := subDirs(t, filepath.Join(root, "app/backend/internal/core"))
	for _, d := range got {
		if !slices.Contains(CoreParts, d) {
			t.Errorf("internal/core/%s is not a core package", d)
		}
	}
	for _, d := range CoreParts {
		if !slices.Contains(got, d) {
			t.Errorf("internal/core/%s is missing", d)
		}
	}
}

func corePackages(t *testing.T, root string) []goPackage {
	t.Helper()
	if !exists(root, "app/backend/internal/core") {
		t.Fatal("app/backend/internal/core does not exist yet")
	}
	mod := modulePath(t, root)
	var out []goPackage
	for _, p := range goPackages(t, root) {
		if m, _, ok := split(mod, p.ImportPath); ok && m == "core" && strings.HasPrefix(p.ImportPath, mod+"/app/") {
			out = append(out, p)
		}
	}
	return out
}

func TestCorePackageSize(t *testing.T) {
	root := repoRoot(t)
	for _, p := range corePackages(t, root) {
		if len(p.GoFiles) > 25 {
			t.Errorf("%s has %d non-test files, the cap is 25", p.ImportPath, len(p.GoFiles))
		}
	}
}

func TestCoreImportsNoModule(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)
	for _, p := range corePackages(t, root) {
		for _, imp := range slices.Concat(p.Imports, p.TestImports, p.XTestImports) {
			if m, _, ok := split(mod, imp); ok && m != "core" {
				t.Errorf("%s imports module %s", p.ImportPath, imp)
			}
		}
	}
}

// testOnlySubs are helper packages another module's tests may import.
var testOnlySubs = []string{"clienttest", "signtest", "fakesite"}

func TestModuleImportsRootOnly(t *testing.T) {
	root := repoRoot(t)
	mod := modulePath(t, root)
	for _, p := range goPackages(t, root) {
		from, _, ok := split(mod, p.ImportPath)
		if !ok {
			continue
		}
		check := func(imps []string, test bool) {
			for _, imp := range imps {
				to, rest, ok := split(mod, imp)
				if !ok || to == from || to == "core" || rest == "" {
					continue
				}
				if test && slices.Contains(testOnlySubs, filepath.Base(rest)) {
					continue
				}
				t.Errorf("%s imports %s (not the module's root package)", p.ImportPath, imp)
			}
		}
		check(p.Imports, false)
		check(slices.Concat(p.TestImports, p.XTestImports), true)
	}
}

var reGoFile = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*(_test)?\.go$`)

func TestGoFileNames(t *testing.T) {
	root := repoRoot(t)
	walkSource(t, root, func(rel string, d fs.DirEntry) {
		if strings.HasSuffix(rel, ".go") && !reGoFile.MatchString(d.Name()) {
			t.Errorf("%s: Go file names are lowercase_with_underscores", rel)
		}
	})
}

var bannedPackageNames = []string{"util", "utils", "common", "shared", "helpers", "misc", "http", "log", "fs", "crypto", "records", "domain"}

func TestNoBannedPackageNames(t *testing.T) {
	root := repoRoot(t)
	for _, sr := range []string{"app/backend", "cmd", "internal"} {
		base := filepath.Join(root, sr)
		if !exists(root, sr) {
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() || p == base {
				return nil
			}
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if slices.Contains(bannedPackageNames, d.Name()) && !bannedNameAllowed[rel] {
				t.Errorf("%s: banned package name", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func isTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".spec.ts")
}

func countLines(t *testing.T, p string) int {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	n := bytes.Count(b, []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func TestFileSizeCap(t *testing.T) {
	root := repoRoot(t)
	walkSource(t, root, func(rel string, d fs.DirEntry) {
		ext := filepath.Ext(rel)
		if (ext != ".go" && ext != ".svelte" && ext != ".ts") || isTestFile(d.Name()) || oversized[rel] {
			return
		}
		if n := countLines(t, filepath.Join(root, rel)); n > 800 {
			t.Errorf("%s: %d lines, the cap is 800", rel, n)
		}
	})
}

var (
	oldPagePaths = []string{"/new", "/status", "/ai-connect", "/reports", "/wishlist", "/ui", "/settings/wishlist"}
	reLiteral    = regexp.MustCompile("\"(?:[^\"\\\\\\n]|\\\\.)*\"|'(?:[^'\\\\\\n]|\\\\.)*'|`[^`]*`")
)

// reAPICall matches the text just before a literal passed to fetch or a
// server-call helper (api.post<T>(, apiGet(, get( ...): an API path, not a
// page link.
var reAPICall = regexp.MustCompile(`(?:\bfetch|\bapi\w*(?:\.\w+)?|\b(?:get|post|put|patch|del|delete))\s*(?:<[^()]*>)?\(\s*$`)

// isOldPage says whether a string literal's text is a whole old page path,
// with or without a query, fragment or further path inside the literal.
func isOldPage(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/api/") || strings.HasPrefix(s, "/v1/") {
		return false
	}
	for _, old := range oldPagePaths {
		rest, ok := strings.CutPrefix(s, old)
		if ok && (rest == "" || rest[0] == '?' || rest[0] == '#' || rest[0] == '/') {
			return true
		}
	}
	return false
}

// TestNoOldPageAddresses: no page link (Go string, or a TS/Svelte string
// not handed to fetch or a server-call helper) and no route folder uses an
// old page address.
func TestNoOldPageAddresses(t *testing.T) {
	root := repoRoot(t)
	for _, base := range []string{"app/frontend/src/routes"} {
		for _, old := range oldPagePaths {
			if exists(root, base+old) {
				t.Errorf("%s%s: route folder at an old page address", base, old)
			}
		}
	}
	walkSource(t, root, func(rel string, d fs.DirEntry) {
		ext := filepath.Ext(rel)
		if (ext != ".go" && ext != ".svelte" && ext != ".ts") || isTestFile(d.Name()) {
			return
		}
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, loc := range reLiteral.FindAllStringIndex(line, -1) {
				lit := line[loc[0]:loc[1]]
				// A line marked "// API path" builds a server path, not a page link.
				if ext != ".go" && (reAPICall.MatchString(line[:loc[0]]) || strings.Contains(line, "// API path")) {
					continue
				}
				if isOldPage(lit[1 : len(lit)-1]) {
					t.Errorf("%s:%d: old page address %s", rel, i+1, lit)
				}
			}
		}
	})
}
