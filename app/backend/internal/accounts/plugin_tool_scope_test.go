package accounts

import (
	"net/http"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestCallerCoversPluginScopeTable(t *testing.T) {
	owner := webserver.Caller{Role: webserver.RoleOwner}
	member := webserver.Caller{Role: "member", Perms: []webserver.Perm{webserver.PermAsk}}
	librarian := webserver.Caller{Role: "member", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	for _, c := range []struct {
		name  string
		who   webserver.Caller
		scope string
		want  bool
	}{
		{"member reads the calendar", member, "calendar.read", true},
		{"member can't read held items", member, "downloads.held.read", false},
		{"member can't change library", member, "library.change", false},
		{"librarian reads held items", librarian, "downloads.held.read", true},
		{"owner changes library", owner, "library.change", true},
		{"no-route scope needs owner", librarian, "requests.users.read", false},
		{"owner holds no-route scope", owner, "requests.users.read", true},
	} {
		if got := CallerCoversPluginScope(c.who, c.scope); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	if len(PluginScopeRoutes("library.read")) <= len(PluginScopeRoutes("library.music.read")) {
		t.Error("a parent scope must open at least its children's routes")
	}
}

func TestPluginBridgeAllowedTable(t *testing.T) {
	owner := webserver.Caller{Role: webserver.RoleOwner}
	member := webserver.Caller{Role: "member", Perms: []webserver.Perm{webserver.PermAsk}}
	for _, c := range []struct {
		name    string
		held    []string
		who     webserver.Caller
		pattern string
		ok      bool
	}{
		{"both hold", []string{"library.read"}, member, "GET /api/v1/titles", true},
		{"plugin lacks scope", []string{"requests.read"}, owner, "GET /api/v1/titles", false},
		{"member lacks route perm", []string{"library.change"}, member, "POST /api/v1/titles", false},
		{"owner with plugin scope", []string{"library.change"}, owner, "POST /api/v1/titles", true},
		{"closed route even for owner", []string{"library.read"}, owner, "GET /api/v1/settings", false},
		{"unknown route", []string{"library.read"}, owner, "GET /api/v1/nope", false},
	} {
		if err := PluginBridgeAllowed(c.held, c.who, c.pattern); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestPluginBearerTable(t *testing.T) {
	for _, c := range []struct{ header, want string }{
		{"Bearer hap_abc", "hap_abc"},
		{"Bearer  hap_abc ", "hap_abc"},
		{"Bearer hk_aikey", ""},
		{"hap_abc", ""},
		{"Basic hap_abc", ""},
		{"", ""},
	} {
		r, _ := http.NewRequest("GET", "/api/v1/titles", nil)
		r.Header.Set("Authorization", c.header)
		if got := pluginBearer(r); got != c.want {
			t.Errorf("%q: %q", c.header, got)
		}
	}
}

func TestPluginCallTokenTable(t *testing.T) {
	IssuePluginToken("lyr", []string{"library.read", "library.change"})
	t.Cleanup(func() { RevokePluginToken("lyr") })
	tok, end := IssuePluginCallToken("lyr", []string{"library.read"})
	who, ok := pluginCaller(tok)
	if !ok || who.PluginID != "lyr" || len(who.Scopes) != 1 || who.Scopes[0] != "library.read" {
		t.Fatalf("call token: %+v %v", who, ok)
	}
	end()
	if _, ok := pluginCaller(tok); ok {
		t.Error("ended call token still works")
	}
	tok2, _ := IssuePluginCallToken("lyr", []string{"library.read"})
	RevokePluginToken("lyr")
	if _, ok := pluginCaller(tok2); ok {
		t.Error("call token outlived the plugin's token")
	}
	if tok3, _ := IssuePluginCallToken("not-running", []string{"library.read"}); func() bool { _, ok := pluginCaller(tok3); return ok }() {
		t.Error("call token for a stopped plugin")
	}
}

func TestPluginForPersonTable(t *testing.T) {
	for _, c := range []struct {
		pattern string
		person  bool
	}{
		{"POST /api/v1/asks", true},
		{"POST /api/v1/asks/{id}/approve", true},
		{"DELETE /api/v1/asks/{id}", true},
		{"PUT /api/v1/ask-limits/{id}", true},
		{"PUT /api/v1/titles/{id}/follow", true},
		{"DELETE /api/v1/metadata/movie/{tmdb_id}/follow", true},
		{"GET /api/v1/my/follows", true},
		{"GET /api/v1/asks", false},
		{"GET /api/v1/titles", false},
		{"POST /api/v1/titles", false},
	} {
		if got := pluginForPerson(c.pattern); got != c.person {
			t.Errorf("%s: %v", c.pattern, got)
		}
	}
}
