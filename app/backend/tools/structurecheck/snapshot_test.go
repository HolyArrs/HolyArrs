package structurecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

var update = flag.Bool("update", false, "rewrite the testdata snapshots")

// repoRoot is the folder holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

// skipDir names folders no snapshot or guard looks inside.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".svelte-kit", "dist", "testdata":
		return true
	}
	return false
}

func compare(t *testing.T, file string, got []string) {
	t.Helper()
	path := filepath.Join("testdata", file)
	body := strings.Join(got, "\n") + "\n"
	if *update {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantLines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(want), "\r\n", "\n"), "\n"), "\n")
	for _, l := range wantLines {
		if !slices.Contains(got, l) {
			t.Errorf("%s: missing now: %s", file, l)
		}
	}
	for _, l := range got {
		if !slices.Contains(wantLines, l) {
			t.Errorf("%s: new or changed: %s", file, l)
		}
	}
}

// TestMigrationsUnchanged: both databases keep the same migrations, in the
// same order, with the same bytes, wherever the files live.
func TestMigrationsUnchanged(t *testing.T) {
	root := repoRoot(t)
	var got []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		db := filepath.Base(filepath.Dir(p))
		if filepath.Base(filepath.Dir(filepath.Dir(p))) != "migrations" || (db != "main" && db != "events") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		b = []byte(strings.ReplaceAll(string(b), "\r\n", "\n"))
		sum := sha256.Sum256(b)
		got = append(got, fmt.Sprintf("%s %s %s", db, d.Name(), hex.EncodeToString(sum[:])))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 50 {
		t.Fatalf("found only %d migration files", len(got))
	}
	slices.Sort(got)
	compare(t, "baseline-migrations.txt", got)
}

var reRegistered = regexp.MustCompile(`Handle(?:Func)?\("((?:GET|HEAD|POST|PUT|PATCH|DELETE) /[^"]*)"`)

// registeredRoutes lists every method-and-path pattern a non-test Go file
// registers on a mux.
func registeredRoutes(t *testing.T, root string) []string {
	t.Helper()
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if strings.Contains(filepath.ToSlash(rel), "/tools/") || strings.HasPrefix(filepath.ToSlash(rel), "tools/") || strings.HasPrefix(filepath.ToSlash(rel), "deploy/platforms/") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range reRegistered.FindAllStringSubmatch(string(b), -1) {
			seen[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	return out
}

// TestRouteTableUnchanged: every route keeps its method, path, permission,
// key scope, fresh-password rule and part.
func TestRouteTableUnchanged(t *testing.T) {
	parts.Register()
	root := repoRoot(t)
	seen := map[string]bool{}
	for _, r := range registeredRoutes(t, root) {
		seen[r] = true
	}
	for r := range accounts.RoutePerms {
		seen[r] = true
	}
	for r := range parts.Routes() {
		seen[r] = true
	}
	if len(seen) < 200 {
		t.Fatalf("found only %d routes", len(seen))
	}
	var got []string
	for r := range seen {
		need, ok := accounts.RoutePerms[r]
		perm := string(need)
		if !ok {
			perm = "unlisted"
		}
		part := "-"
		if o, ok := parts.RouteOwner(r); ok {
			part = string(o)
		}
		got = append(got, fmt.Sprintf("%s auth=%s scope=%s assurance=%t part=%s",
			r, perm, accounts.RouteScope(r), accounts.SessionNeedsRecentLocal(r), part))
	}
	slices.Sort(got)
	compare(t, "route-table.txt", got)
}
