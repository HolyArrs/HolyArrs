// Package routehits records which mux routes test servers actually served,
// so a test suite can prove every /api/v1 route was called by at least one
// test request. Only tests import it.
package webserver

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var (
	mu   sync.Mutex
	hits = map[string]bool{}
)

// Wrap records the pattern of every request h hands to a ServeMux. Put it
// right outside the mux (or outside a handler that passes the same request
// to the mux): the mux writes the matched pattern into the request, so a
// request that never reached the mux (refused before it) is not counted.
func RecordRouteHits(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if r.Pattern != "" {
			mu.Lock()
			hits[r.Pattern] = true
			mu.Unlock()
		}
	})
}

var reRoute = regexp.MustCompile(`HandleFunc\("([A-Z]+ /api/v1/[^"]*)"`)

// RegisteredRoutes lists the /api/v1 routes the non-test Go files in dir register.
func RegisteredRoutes(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, m := range reRoute.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out, nil
}

// UnhitRoutes lists the routes in want that no test request reached.
func UnhitRoutes(want []string) []string {
	mu.Lock()
	defer mu.Unlock()
	var out []string
	for _, r := range want {
		if !hits[r] {
			out = append(out, r)
		}
	}
	return out
}

// Check is for TestMain after m.Run: when the whole package ran (no -run,
// -skip or -short), every /api/v1 route registered in dir must have been
// served at least once. It returns the exit code to use.
func CheckRouteHits(code int, dir string) int {
	if code != 0 {
		return code
	}
	for _, f := range []string{"test.run", "test.skip"} {
		if fl := flag.Lookup(f); fl != nil && fl.Value.String() != "" {
			return code
		}
	}
	routes, err := RegisteredRoutes(dir)
	if err != nil || len(routes) == 0 {
		fmt.Fprintln(os.Stderr, "routehits: no routes found:", err)
		return 1
	}
	if u := UnhitRoutes(routes); len(u) > 0 {
		fmt.Fprintf(os.Stderr, "routehits: %d of %d routes were never served to a test request:\n  %s\n", len(u), len(routes), strings.Join(u, "\n  "))
		return 1
	}
	return code
}
