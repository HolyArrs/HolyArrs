package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// An API key has no current session, so "sign out my other sessions" would
// end every one: it is refused with a plain reason.
func TestEndOtherSessionsRefusesAPIKey(t *testing.T) {
	a := &Auth{}
	for _, via := range []string{"api_key", "local_network"} {
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: via, Scopes: []string{webserver.ScopeAll}}))
		w := httptest.NewRecorder()
		a.endMyOtherSessions(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "use a signed-in browser session") {
			t.Fatalf("%s: %d %s", via, w.Code, w.Body.String())
		}
	}
}
