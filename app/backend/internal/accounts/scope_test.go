package accounts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// allRoutes is every route the server registers on its mux: the /api/v1
// routes of the packages that register them, and every RoutePerms entry
// (pages and the live feed included).
func allRoutes(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, dir := range []string{".", "../api", "../backups", "../alerts", "../health", "../updates", "../mediaservers", "../metadata", "../sources"} {
		rs, err := webserver.RegisteredRoutes(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			seen[r] = true
		}
	}
	for r := range RoutePerms {
		seen[r] = true
	}
	if len(seen) < 200 {
		t.Fatalf("found only %d routes", len(seen))
	}
	var out []string
	for r := range seen {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// TestEveryRouteChecksItsScope walks the whole route table: for each
// route and each key scope, an owner's key gets through the Gate exactly
// when its scopes cover the route's scope. The owner holds every
// permission, so a refusal here is the scope's alone.
func TestEveryRouteChecksItsScope(t *testing.T) {
	routes := allRoutes(t)
	mux := http.NewServeMux()
	for _, r := range routes {
		mux.HandleFunc(r, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	}
	gate := Gate(mux)
	param := regexp.MustCompile(`\{[^}]+\}`)
	keys := [][]string{{webserver.ScopeRead}, {webserver.ScopeWrite}, {webserver.ScopeAdmin}, {webserver.ScopeControl}, {webserver.ScopeAll}, {webserver.ScopeRead, webserver.ScopeControl}}
	counts := map[string]int{}
	for _, r := range routes {
		scope := RouteScope(r)
		if !slices.Contains(Scopes, scope) || scope == webserver.ScopeAll {
			t.Fatalf("%s: scope %q is not one a route can need", r, scope)
		}
		counts[scope]++
		method, path, _ := strings.Cut(r, " ")
		path = param.ReplaceAllString(path, "1")
		for _, held := range keys {
			who := webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "api_key", Scopes: held}
			req := httptest.NewRequest(method, path, nil).WithContext(webserver.ContextWith(context.Background(), who))
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			want := webserver.ScopesCover(held, scope)
			got := rec.Code == http.StatusTeapot
			if want != got {
				t.Errorf("%s (needs %s) with key %v: got %d, want through=%v", r, scope, held, rec.Code, want)
			}
			if !got && (rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "scope")) {
				t.Errorf("%s with key %v: refused with %d %s, not the scope answer", r, held, rec.Code, rec.Body)
			}
		}
		// A session holds every scope.
		req := httptest.NewRequest(method, path, nil).WithContext(webserver.ContextWith(context.Background(), webserver.Caller{
			UserID: 1, Role: webserver.RoleOwner, Via: "session", AuthMethod: AuthMethodLocalPassword, LocalRecent: true,
		}))
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if rec.Code != http.StatusTeapot {
			t.Errorf("%s with a session: %d", r, rec.Code)
		}
	}
	for _, sc := range []string{webserver.ScopeRead, webserver.ScopeWrite, webserver.ScopeAdmin, webserver.ScopeControl} {
		if counts[sc] == 0 {
			t.Errorf("no route needs %s", sc)
		}
	}
}

// TestKeyScopeCannotBeRaised: no route changes a key once made, and the
// key routes themselves need the admin scope.
func TestKeyScopeCannotBeRaised(t *testing.T) {
	for _, r := range allRoutes(t) {
		_, path, _ := strings.Cut(r, " ")
		if strings.HasPrefix(path, "/api/v1/auth/keys") {
			if strings.HasPrefix(r, "PUT ") || strings.HasPrefix(r, "PATCH ") {
				t.Errorf("%s edits a key; a key's scopes are fixed when it is made", r)
			}
			if RouteScope(r) != webserver.ScopeAdmin {
				t.Errorf("%s needs %s, want admin", r, RouteScope(r))
			}
		}
	}
	// What a key may hold when it makes another.
	cases := []struct {
		held []string
		want string
		ok   bool
	}{
		{[]string{webserver.ScopeAll}, webserver.ScopeRead, true},
		{[]string{webserver.ScopeAll}, webserver.ScopeAdmin, true},
		{[]string{webserver.ScopeAll}, webserver.ScopeControl, false},
		{[]string{webserver.ScopeAdmin}, webserver.ScopeAll, false},
		{[]string{webserver.ScopeAdmin}, webserver.ScopeWrite, false},
		{[]string{webserver.ScopeRead, webserver.ScopeWrite, webserver.ScopeAdmin}, webserver.ScopeAll, false},
		{[]string{webserver.ScopeControl}, webserver.ScopeRead, false},
	}
	for _, c := range cases {
		if got := webserver.ScopesCover(c.held, c.want); got != c.ok {
			t.Errorf("held %v asking %s: %v, want %v", c.held, c.want, got, c.ok)
		}
	}
}
