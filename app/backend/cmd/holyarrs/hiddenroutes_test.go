package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestNoHiddenRoutes: the spec generator reads the routes from the source
// (routehits.Registered over routePackages), so a route registered any
// other way would be missing from openapi.json and from the scope tests.
// Every mux registration in the program must be a string literal, and
// every /api/ route must sit in one of routePackages in the form the
// generator reads.
func TestNoHiddenRoutes(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	call := regexp.MustCompile(`\b(\w+)\.(HandleFunc|Handle)\(\s*([^,)]*)`)
	read := regexp.MustCompile(`^"([A-Z]+ /api/v1/[^"]*)"$`)
	var dirs []string
	for _, d := range routePackages {
		abs, _ := filepath.Abs(d)
		dirs = append(dirs, abs)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "web" || strings.HasPrefix(d.Name(), ".")) && path != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.ToSlash(path) == "../../internal/core/webserver/router.go" {
			return nil // the router's own plumbing: it takes the pattern from its callers
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		absDir, _ := filepath.Abs(filepath.Dir(path))
		for _, m := range call.FindAllStringSubmatch(string(b), -1) {
			recv, arg := m[1], strings.TrimSpace(m[3])
			if (recv == "q" && m[2] == "Handle") || recv == "windows" {
				continue // Job handlers and Windows file handles, not routes
			}
			if !strings.HasPrefix(arg, `"`) {
				t.Errorf("%s: %s.%s(%s, ...) registers a route the spec generator cannot see; use a string literal", path, recv, m[2], arg)
				continue
			}
			if !strings.Contains(arg, "/api/") {
				continue
			}
			if !read.MatchString(arg) {
				t.Errorf("%s: route %s is not in the METHOD /api/v1/... form the generator reads", path, arg)
			}
			if !slices.Contains(dirs, absDir) {
				t.Errorf("%s: route %s is registered outside routePackages %v", path, arg, routePackages)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
