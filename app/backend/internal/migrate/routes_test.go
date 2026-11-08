package migrate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestMigrateRoutesWithoutVault(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)

	owner := webserver.ContextWith(httptest.NewRequest("GET", "/", nil).Context(), webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(owner)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// The secrets-file routes are left out without a key store.
	if c := do("GET", "/api/v1/migrate/saved-keys", "").Code; c != http.StatusNotFound && c != http.StatusMethodNotAllowed {
		t.Errorf("saved-keys without a vault: %d, want 404 or 405", c)
	}
	// A saved key cannot exist without a vault: a clear refusal, no panic.
	for _, path := range []string{"/api/v1/migrate/read", "/api/v1/migrate/probe"} {
		rec := do("POST", path, `{"kind":"radarr","use_saved_key":true}`)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
			t.Errorf("%s with a saved key and no vault: %d %s, want 400 or 403", path, rec.Code, rec.Body.String())
		}
	}
}
