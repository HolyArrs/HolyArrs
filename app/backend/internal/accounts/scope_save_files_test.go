package accounts

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Saving a Library file to a device: save_files (the owner holds it by
// role), never for a Kid, an API key needs the read scope, and no plugin
// reaches it whatever it was given.
func TestSaveFileRouteGate(t *testing.T) {
	const route = "GET /api/v1/titles/{id}/files/{fid}/save"
	if RoutePerms[route] != webserver.PermSaveFiles || RouteScope(route) != webserver.ScopeRead || !PluginClosed[route] {
		t.Fatalf("perm %q scope %q closed to plugins %v", RoutePerms[route], RouteScope(route), PluginClosed[route])
	}
	m := http.NewServeMux()
	m.HandleFunc(route, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	gate := Gate(m)
	save := []webserver.Perm{webserver.PermAsk, webserver.PermSaveFiles}
	for _, c := range []struct {
		name string
		who  webserver.Caller
		want int
	}{
		{"owner", webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner}, 200},
		{"admin without it", webserver.Caller{UserID: 2, Via: "session", Role: webserver.RoleAdmin, Perms: webserver.DefaultPerms(webserver.RoleAdmin)}, 403},
		{"member without it", webserver.Caller{UserID: 3, Via: "session", Role: webserver.RoleMember, Perms: webserver.DefaultPerms(webserver.RoleMember)}, 403},
		{"member given it", webserver.Caller{UserID: 3, Via: "session", Role: webserver.RoleMember, Perms: save}, 200},
		{"kid given it", webserver.Caller{UserID: 4, Via: "session", Role: webserver.RoleMember, Perms: save, MaxRating: "PG"}, 403},
		{"read key", webserver.Caller{UserID: 3, Via: "api_key", Role: webserver.RoleMember, Perms: save, Scopes: []string{webserver.ScopeRead}}, 200},
		{"all key", webserver.Caller{UserID: 3, Via: "api_key", Role: webserver.RoleMember, Perms: save, Scopes: []string{webserver.ScopeAll}}, 200},
		{"write key", webserver.Caller{UserID: 3, Via: "api_key", Role: webserver.RoleMember, Perms: save, Scopes: []string{webserver.ScopeWrite}}, 403},
		{"health key", webserver.Caller{UserID: 1, Via: "api_key", Role: webserver.RoleOwner, Scopes: []string{webserver.ScopeHealthRead}}, 403},
		{"read key without it", webserver.Caller{UserID: 3, Via: "api_key", Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk}, Scopes: []string{webserver.ScopeRead}}, 403},
		{"plugin", webserver.Caller{Via: "plugin", PluginID: "p", Scopes: []string{"library.read", "library.change", "media.stream"}}, 403},
	} {
		req := httptest.NewRequest("GET", "/api/v1/titles/1/files/2/save", nil)
		req = req.WithContext(webserver.ContextWith(req.Context(), c.who))
		w := httptest.NewRecorder()
		gate.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("%s: %d, want %d: %s", c.name, w.Code, c.want, w.Body.String())
		}
		head := httptest.NewRequest("HEAD", "/api/v1/titles/1/files/2/save", nil)
		head = head.WithContext(webserver.ContextWith(head.Context(), c.who))
		w = httptest.NewRecorder()
		gate.ServeHTTP(w, head)
		if w.Code != c.want {
			t.Errorf("%s HEAD: %d, want %d", c.name, w.Code, c.want)
		}
	}
}
