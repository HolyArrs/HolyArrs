package accounts

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TestEveryScopeEnforcedPerRoute walks every route a plugin scope opens.
// For each one a plugin holding exactly that scope gets through the Gate (or
// the person-only refusal), and plugins holding nothing, a sibling scope or
// the other verb of the same area are refused with 403.
func TestEveryScopeEnforcedPerRoute(t *testing.T) {
	param := regexp.MustCompile(`\{[^}]+\}`)
	reached := ""
	m := http.NewServeMux()
	for r := range PluginScopes {
		m.HandleFunc(r, func(w http.ResponseWriter, req *http.Request) { reached = r; w.WriteHeader(http.StatusTeapot) })
	}
	g := Gate(m)
	call := func(pattern string, scopes []string) int {
		method, path, _ := strings.Cut(pattern, " ")
		path = strings.ReplaceAll(param.ReplaceAllString(path, "1"), "1...", "1")
		id := "every-" + strings.ReplaceAll(strings.Join(scopes, "-"), ".", "_")
		if len(id) > 60 {
			id = id[:60]
		}
		IssuePluginToken(id, scopes)
		defer RevokePluginToken(id)
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req = req.WithContext(webserver.ContextWith(req.Context(), webserver.Caller{Via: "plugin", PluginID: id, Scopes: scopes}))
		w := httptest.NewRecorder()
		reached = ""
		g.ServeHTTP(w, req)
		return w.Code
	}
	other := func(scope string) string {
		if strings.HasSuffix(scope, ".read") {
			return strings.TrimSuffix(scope, ".read") + ".change"
		}
		return strings.TrimSuffix(scope, scope[strings.LastIndexByte(scope, '.'):]) + ".read"
	}
	scopesSeen := map[string]bool{}
	for pattern, scope := range PluginScopes {
		scopesSeen[scope] = true
		code := call(pattern, []string{scope})
		switch {
		case pluginForPerson(pattern):
			if code != http.StatusForbidden {
				t.Errorf("%s: acts for a person, got %d", pattern, code)
			}
		case strings.HasPrefix(scope, "files.") && !strings.HasSuffix(scope, ".read") && PluginHandsOff != nil && PluginHandsOff():
		default:
			if code != http.StatusTeapot || reached != pattern {
				t.Errorf("%s with %s: %d (reached %q)", pattern, scope, code, reached)
			}
		}
		for _, held := range [][]string{nil, {"calendar.read"}, {other(scope)}} {
			if len(held) == 1 && PluginScopesCover(held, scope) {
				continue
			}
			if c := call(pattern, held); c != http.StatusForbidden {
				t.Errorf("%s holding %v: %d, want 403", pattern, held, c)
			}
		}
	}
	for _, s := range PluginScopeList() {
		if !scopesSeen[s] && len(PluginScopeRoutes(s)) > 0 {
			t.Errorf("%s opens routes but none was walked", s)
		}
	}
	if len(scopesSeen) < 70 {
		t.Fatalf("only %d scopes have routes", len(scopesSeen))
	}
}
