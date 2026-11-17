package mcpkeys

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/route-groups.txt")

var registration = regexp.MustCompile(`Handle(?:Func)?\("([A-Z]+ /[^"]*)"`)

// appRoutes reads every route pattern the shipped Go files register.
func appRoutes(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, root := range []string{"../..", "../../../cmd"} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range registration.FindAllStringSubmatch(string(b), -1) {
				seen[m[1]] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) < 400 {
		t.Fatalf("found only %d routes", len(seen))
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	return out
}

// TestEveryRouteHasAGroup keeps new routes from slipping past the tick boxes.
func TestEveryRouteHasAGroup(t *testing.T) {
	counts := map[Group]int{}
	var lines []string
	for _, r := range appRoutes(t) {
		method, _, _ := strings.Cut(r, " ")
		g, ok := GroupFor(method, r)
		if !ok {
			t.Errorf("%s has no group: add a rule in groups.go", r)
			continue
		}
		counts[g]++
		lines = append(lines, r+" "+string(g))
		if !Allows(Set{g}, method, r) || (g != See && Allows(Set{See}, method, r)) {
			t.Errorf("%s: Allows disagrees with group %s", r, g)
		}
	}
	for _, g := range All {
		if counts[g] == 0 {
			t.Errorf("group %s holds no route", g)
		}
	}
	t.Logf("routes per group: %v", counts)
	slices.Sort(lines)
	compareSnapshot(t, lines)
}

// compareSnapshot pins every route's group in testdata/route-groups.txt, so a
// new or regrouped route is a reviewed change, never a silent one.
func compareSnapshot(t *testing.T, got []string) {
	t.Helper()
	path := filepath.Join("testdata", "route-groups.txt")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantLines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(want), "\r\n", "\n"), "\n"), "\n")
	const hint = "review the route's group in groups.go, then run go test ./internal/ai/mcpkeys/ -run TestEveryRouteHasAGroup -update"
	for _, l := range wantLines {
		if !slices.Contains(got, l) {
			t.Errorf("route-groups.txt: removed or changed: %s (%s)", l, hint)
		}
	}
	for _, l := range got {
		if !slices.Contains(wantLines, l) {
			t.Errorf("route-groups.txt: new or changed: %s (%s)", l, hint)
		}
	}
}

func TestGroupFor(t *testing.T) {
	for _, c := range []struct {
		method, pattern string
		want            Group
	}{
		{"GET", "/api/v1/titles", See},
		{"POST", "/api/v1/asks", Add},
		{"POST", "/api/v1/downloads/{id}/pause", Control},
		{"POST", "/api/v1/downloads/{id}/remove", Delete},
		{"POST", "/api/v1/titles/{id}/delete", Delete},
		{"DELETE", "/api/v1/sources/{id}", Delete},
		{"PUT", "/api/v1/settings/naming", Settings},
		{"GET", "/api/v1/settings/naming", Settings},
		{"POST", "/api/v1/auth/keys", Admin},
		{"POST", "/api/v1/backups/restore", Admin},
		{"POST", "/api/v1/titles/{id}/search", Control},
	} {
		if g, ok := GroupFor(c.method, c.pattern); !ok || g != c.want {
			t.Errorf("%s %s = %q %v, want %q", c.method, c.pattern, g, ok, c.want)
		}
	}
}

func TestAllowsRefusesUnknown(t *testing.T) {
	all := Set(All)
	for _, c := range [][2]string{
		{"GET", "/api/v2/titles"},
		{"GET", "/secret"},
		{"OPTIONS", "/api/v1/titles"},
		{"GET", "POST /api/v1/titles"},
		{"GET", "api/v1/titles"},
	} {
		if Allows(all, c[0], c[1]) {
			t.Errorf("%s %s allowed", c[0], c[1])
		}
	}
	if Allows(nil, "GET", "/api/v1/titles") {
		t.Error("empty set allowed a route")
	}
}

// The AI tools check real paths; pinned {id} routes must match them.
func TestOverridesMatchRealPaths(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         Group
	}{
		{"POST", "/api/v1/asks/3/approve", Approve},
		{"POST", "/api/v1/asks/3/turn-down", Approve},
		{"PUT", "/api/v1/downloaders/5", Settings},
		{"POST", "/api/v1/downloaders/5/test", Settings},
		{"POST", "/api/v1/collections/5/remove/preview", See},
		{"POST", "/api/v1/asks//approve", Add},
	} {
		if g, _ := GroupFor(c.method, c.path); g != c.want {
			t.Errorf("%s %s: %s, want %s", c.method, c.path, g, c.want)
		}
	}
}
