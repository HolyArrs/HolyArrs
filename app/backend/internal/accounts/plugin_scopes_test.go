package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TestEveryRouteMappedOrClosedForPlugins: a new route fails here until
// someone decides whether a plugin may call it.
func TestEveryRouteMappedOrClosedForPlugins(t *testing.T) {
	for _, r := range allRoutes(t) {
		_, mapped := PluginScopes[r]
		closed := PluginClosed[r]
		if mapped == closed {
			t.Errorf("%s: list it in exactly one of the plugin scope map or the closed list (plugin_scopes.json)", r)
		}
	}
	for r := range PluginClosed {
		if _, ok := PluginScopes[r]; ok {
			t.Errorf("%s: closed and mapped at once", r)
		}
	}
}

// TestClosedAreasNeverMapped keeps the parts that keep control out of reach
// at any scope count.
func TestClosedAreasNeverMapped(t *testing.T) {
	closedPrefixes := []string{
		"/api/v1/plugins", "/api/v1/plugin-images", "/api/v1/auth", "/api/v1/users", "/api/v1/my/",
		"/api/v1/settings", "/api/v1/app/", "/api/v1/parts", "/api/v1/stages", "/api/v1/backups",
		"/api/v1/migrate", "/api/v1/bridges", "/api/v1/mcp", "/mcp", "/live", "/api/v1/openapi",
		"/api/v1/logs/", "/api/v1/support", "/sonarr/", "/radarr/", "/oauth", "/.well-known",
	}
	for r := range PluginScopes {
		_, path, _ := strings.Cut(r, " ")
		for _, p := range closedPrefixes {
			if strings.HasPrefix(path, p) {
				t.Errorf("%s is mapped to %s but sits in a closed area", r, PluginScopes[r])
			}
		}
		if strings.HasSuffix(path, "/restart") || strings.HasSuffix(path, "/test") {
			t.Errorf("%s is mapped but restarts or tests a connection", r)
		}
	}
}

// TestPluginScopeCatalog: about 90 names, each in one of the 8 groups.
func TestPluginScopeCatalog(t *testing.T) {
	list := PluginScopeList()
	if len(list) < 80 {
		t.Fatalf("only %d plugin scopes", len(list))
	}
	if len(PluginGroups()) != 8 {
		t.Fatalf("%d groups, want 8", len(PluginGroups()))
	}
	for _, s := range list {
		if PluginScopeGroup(s) == "" {
			t.Errorf("%s has no group", s)
		}
	}
}

func TestPluginScopeTree(t *testing.T) {
	for _, c := range []struct {
		held []string
		need string
		want bool
	}{
		{[]string{"library.read"}, "library.read", true},
		{[]string{"library.read"}, "library.music.read", true},
		{[]string{"library.music.read"}, "library.read", false},
		{[]string{"library.read"}, "library.change", false},
		{[]string{"library.change"}, "library.read", false},
		{[]string{"library.read"}, "libraryx.read", false},
		{[]string{"library.music.read"}, "library.metadata.read", false},
		{[]string{"downloads.control"}, "downloads.clients.control", true},
		{nil, "library.read", false},
		{[]string{"library.read"}, "", false},
	} {
		if got := PluginScopesCover(c.held, c.need); got != c.want {
			t.Errorf("held %v need %q: got %v want %v", c.held, c.need, got, c.want)
		}
	}
}

func TestValidPluginScope(t *testing.T) {
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"library.read", true},
		{"library.music.read", true},
		{"files.read", true}, // a parent of files.folders.read
		{"network.forward", true},
		{"settings.read", false},
		{"plugins.change", false},
		{"auth.read", false},
		{"library", false},
		{"", false},
		{"library.read.extra", false},
		{"LIBRARY.read", false},
	} {
		if got := ValidPluginScope(c.s); got != c.want {
			t.Errorf("%q: got %v want %v", c.s, got, c.want)
		}
	}
}

func TestPluginTokens(t *testing.T) {
	tok := IssuePluginToken("demo", []string{"library.read"})
	if tok == "" {
		t.Fatal("no token")
	}
	who, ok := pluginCaller(tok)
	if !ok || who.Via != "plugin" || who.PluginID != "demo" {
		t.Fatalf("caller %+v %v", who, ok)
	}
	again := IssuePluginToken("demo", []string{"library.read"})
	if _, ok := pluginCaller(tok); ok {
		t.Error("old token still works after a new start")
	}
	RevokePluginToken("demo")
	if _, ok := pluginCaller(again); ok {
		t.Error("token works after stop")
	}
	if _, ok := pluginCaller(""); ok {
		t.Error("empty token accepted")
	}
}

// TestGatePluginCaller: a plugin reaches only routes its scopes cover,
// whatever its role, and never a closed or unknown route.
func TestGatePluginCaller(t *testing.T) {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }
	for _, p := range []string{"GET /api/v1/titles", "GET /api/v1/titles/{id}/albums", "GET /api/v1/settings/ui", "POST /api/v1/auth/password", "GET /api/v1/made-up"} {
		mux.HandleFunc(p, ok)
	}
	g := Gate(mux)
	IssuePluginToken("demo", []string{"library.read"})
	t.Cleanup(func() { RevokePluginToken("demo") })
	plugin := webserver.Caller{Via: "plugin", PluginID: "demo", Scopes: []string{"library.read"}}
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/v1/titles", http.StatusTeapot},
		{"GET", "/api/v1/settings/ui", http.StatusForbidden},
		{"POST", "/api/v1/auth/password", http.StatusForbidden},
		{"GET", "/api/v1/made-up", http.StatusForbidden},
	} {
		r := httptest.NewRequest(c.method, c.path, nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), plugin))
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s: %d want %d", c.method, c.path, w.Code, c.want)
		}
	}
	none := plugin
	none.Scopes = nil
	r := httptest.NewRequest("GET", "/api/v1/titles", nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), none))
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("no scope: %d want 403", w.Code)
	}
}

// TestPluginHoldsNoLegacyScope: handlers that check the key scopes never
// treat a plugin as a session.
func TestPluginHoldsNoLegacyScope(t *testing.T) {
	c := webserver.Caller{Via: "plugin", Scopes: []string{"library.read"}}
	for _, s := range []string{webserver.ScopeRead, webserver.ScopeAdmin, webserver.ScopeWrite, webserver.ScopeControl} {
		if c.HasScope(s) {
			t.Errorf("plugin holds %s", s)
		}
	}
	if webserver.Strong(webserver.ContextWith(t.Context(), c)) {
		t.Error("plugin counts as strong")
	}
}

func TestPluginRateAndCost(t *testing.T) {
	IssuePluginToken("rate", nil)
	t.Cleanup(func() { RevokePluginToken("rate") })
	now := time.Now()
	for _, c := range []struct {
		cost, want int
	}{
		{pluginRate - 120, callOK},
		{120, callOK},
		{1, callTooMany},
		{0, callTooMany},
	} {
		if got := pluginAllowCall("rate", c.cost, now); got != c.want {
			t.Fatalf("cost %d: %d want %d", c.cost, got, c.want)
		}
	}
	if got := pluginAllowCall("rate", 1, now.Add(time.Minute)); got != callOK {
		t.Errorf("next minute: %d", got)
	}
	if got := pluginAllowCall("gone", 1, now); got != callRevoked {
		t.Errorf("no token: %d", got)
	}
}

func TestPluginHandsOffRefusesFileChanges(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/rename", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	mux.HandleFunc("GET /api/v1/rename", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	IssuePluginToken("ho", []string{"files.change", "files.read"})
	t.Cleanup(func() { RevokePluginToken("ho"); PluginHandsOff = nil })
	who := webserver.Caller{Via: "plugin", PluginID: "ho", Scopes: []string{"files.change", "files.read"}}
	for _, c := range []struct {
		on     bool
		method string
		want   int
	}{
		{false, "POST", http.StatusTeapot},
		{true, "POST", http.StatusForbidden},
		{true, "GET", http.StatusTeapot},
	} {
		on := c.on
		PluginHandsOff = func() bool { return on }
		r := httptest.NewRequest(c.method, "/api/v1/rename", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), who))
		w := httptest.NewRecorder()
		Gate(mux).ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("hands-off %v %s: %d want %d", c.on, c.method, w.Code, c.want)
		}
	}
}

// TestPluginTokenOnlyFromLoopback: a copied token is refused from any other
// address, and loopback loses the local-network skip while plugins run.
func TestPluginTokenOnlyFromLoopback(t *testing.T) {
	a := setup(t, Options{}).a
	tok := IssuePluginToken("lb", []string{"library.read"})
	t.Cleanup(func() { RevokePluginToken("lb") })
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, _ := webserver.From(r.Context())
		if who.Via != "plugin" {
			t.Errorf("via %q", who.Via)
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	for _, c := range []struct {
		remote string
		want   int
	}{
		{"127.0.0.1:5000", http.StatusTeapot},
		{"[::1]:5000", http.StatusTeapot},
		{"192.168.1.20:5000", http.StatusUnauthorized},
		{"100.87.1.2:5000", http.StatusUnauthorized},
	} {
		r := httptest.NewRequest("GET", "/api/v1/titles", nil)
		r.RemoteAddr = c.remote
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: %d want %d", c.remote, w.Code, c.want)
		}
	}
	r := httptest.NewRequest("GET", "/api/v1/titles", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	a.skipLocal.Store(true)
	if a.strictLocal(r) {
		t.Error("loopback got the local-network skip while a plugin runs")
	}
}
