package parts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const webRoutes = "../../web/app/src/routes"

// Every page of the web app (each +page.svelte) and every menu entry of
// the app frame belongs to a part or is marked core in Screens; a row for a
// page that is gone fails too.
func TestEveryScreenHasAPart(t *testing.T) {
	pages := map[string]bool{}
	err := filepath.WalkDir(webRoutes, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "+page.svelte" {
			return err
		}
		rel, _ := filepath.Rel(webRoutes, filepath.Dir(p))
		path := "/" + strings.TrimPrefix(filepath.ToSlash(rel), ".")
		path = strings.TrimSuffix(path, "/")
		if path == "" {
			path = "/"
		}
		pages[path] = true
		if _, ok := Screens[path]; !ok {
			t.Errorf("page %s belongs to no part and is not marked core in internal/parts/table.go", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 30 {
		t.Fatalf("found only %d pages", len(pages))
	}
	for s := range Screens {
		if !pages[s] {
			t.Errorf("Screens lists %s, but there is no such page", s)
		}
	}
	layout, err := os.ReadFile(filepath.Join(webRoutes, "+layout.svelte"))
	if err != nil {
		t.Fatal(err)
	}
	hrefs := regexp.MustCompile(`href: '([^']+)'`).FindAllStringSubmatch(string(layout), -1)
	if len(hrefs) < 6 {
		t.Fatalf("found only %d menu entries", len(hrefs))
	}
	for _, m := range hrefs {
		if _, ok := ScreenOwner(m[1]); !ok {
			t.Errorf("menu entry %s belongs to no part", m[1])
		}
	}
}
