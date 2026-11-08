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
	routeHitsMu sync.Mutex
	routeHits   = map[string]bool{}
)

var routePattern = regexp.MustCompile(`Handle(?:Func)?\("([A-Z]+ /api/v1/[^"]*)"`)

// RecordRouteHits notes the mux pattern of every request that h serves.
// Put it right outside the mux: the mux writes the matched pattern into the
// request, so a request refused before the mux is not counted.
func RecordRouteHits(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if r.Pattern != "" {
			routeHitsMu.Lock()
			routeHits[r.Pattern] = true
			routeHitsMu.Unlock()
		}
	})
}

// RegisteredRoutes lists the /api/v1 routes that the non-test Go files in
// dir register.
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
		for _, m := range routePattern.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out, nil
}

// UnhitRoutes lists the routes in want that no request has reached.
func UnhitRoutes(want []string) []string {
	routeHitsMu.Lock()
	defer routeHitsMu.Unlock()
	var out []string
	for _, r := range want {
		if !routeHits[r] {
			out = append(out, r)
		}
	}
	return out
}

// CheckRouteHits is for TestMain, after m.Run. When the whole package ran
// (no -run, -skip or -short), every /api/v1 route registered in dir must have
// been served at least once. It returns the exit code to use.
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
		fmt.Fprintln(os.Stderr, "webserver: route hits: no routes found:", err)
		return 1
	}
	if u := UnhitRoutes(routes); len(u) > 0 {
		fmt.Fprintf(os.Stderr, "webserver: route hits: %d of %d routes were never served to a test request:\n  %s\n", len(u), len(routes), strings.Join(u, "\n  "))
		return 1
	}
	return code
}
