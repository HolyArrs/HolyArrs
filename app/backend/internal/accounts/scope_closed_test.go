package accounts

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// neverRead lists GET routes that hand out the database, secrets, hashes,
// keys, sign-in settings or the people list. A read key must never reach
// them; each needs the admin scope.
var neverRead = []string{
	"GET /api/v1/backups",
	"GET /api/v1/backups/{name}", // the whole database, password hashes included
	"GET /api/v1/auth/keys",
	"GET /api/v1/auth/settings",
	"GET /api/v1/settings/tmdb-token",
	"GET /api/v1/settings/trakt-key",
	"GET /api/v1/settings/trakt-token",
	"GET /api/v1/users",
	"GET /api/v1/users/invites",
	"GET /api/v1/ask-limits",
}

func TestSecretRoutesNeedAdmin(t *testing.T) {
	for _, r := range neverRead {
		if got := RouteScope(r); got != webserver.ScopeAdmin {
			t.Errorf("%s needs %q, want admin", r, got)
		}
		if ReadRoutes[r] {
			t.Errorf("%s is listed in ReadRoutes", r)
		}
	}
}

// TestEveryGETHasAnExplicitScope: a GET route is read only when ReadRoutes
// lists it; one in neither ReadRoutes nor RouteScopes is a route nobody
// decided on. RouteScope gives such a route admin (fail closed), and this
// test fails so the decision is made on purpose.
func TestEveryGETHasAnExplicitScope(t *testing.T) {
	for _, r := range allRoutes(t) {
		if !strings.HasPrefix(r, "GET ") && !strings.HasPrefix(r, "HEAD ") {
			continue
		}
		_, inRead := ReadRoutes[r]
		_, inScopes := RouteScopes[r]
		if inRead == inScopes {
			t.Errorf("%s: list it in exactly one of ReadRoutes (read) or RouteScopes (admin or another scope)", r)
		}
	}
	for r := range ReadRoutes {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("%s: ReadRoutes holds only GET routes", r)
		}
	}
}

func TestUnlistedGETFailsClosed(t *testing.T) {
	if got := RouteScope("GET /api/v1/made-up-new-route"); got != webserver.ScopeAdmin {
		t.Errorf("an unlisted GET needs %q, want admin", got)
	}
}
