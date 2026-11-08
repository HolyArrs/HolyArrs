package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestHealthReadScopeOnlyReachesHealthStates(t *testing.T) {
	mux := http.NewServeMux()
	for _, pattern := range []string{
		"GET /api/v1/health/states",
		"GET /api/v1/titles",
		"GET /api/v1/settings/health",
		"GET /api/v1/auth/keys",
		"POST /api/v1/health/checks/run",
		"POST /api/v1/users/invites",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	}
	gate := Gate(mux)
	who := webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "api_key", Scopes: []string{webserver.ScopeHealthRead}}

	cases := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/api/v1/health/states", http.StatusTeapot},
		{http.MethodGet, "/api/v1/titles", http.StatusForbidden},
		{http.MethodGet, "/api/v1/settings/health", http.StatusForbidden},
		{http.MethodGet, "/api/v1/auth/keys", http.StatusForbidden},
		{http.MethodPost, "/api/v1/health/checks/run", http.StatusForbidden},
		{http.MethodPost, "/api/v1/users/invites", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req = req.WithContext(webserver.ContextWith(req.Context(), who))
			rec := httptest.NewRecorder()
			gate.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestBroadReadScopesStillReachHealthStates(t *testing.T) {
	if got := RouteScope("GET /api/v1/health/states"); got != webserver.ScopeHealthRead {
		t.Fatalf("health states scope = %q, want %q", got, webserver.ScopeHealthRead)
	}
	for _, held := range [][]string{{webserver.ScopeHealthRead}, {webserver.ScopeRead}, {webserver.ScopeAll}} {
		if !webserver.ScopesCover(held, webserver.ScopeHealthRead) {
			t.Errorf("scope %v no longer covers health states", held)
		}
	}
	for _, requested := range []string{webserver.ScopeRead, webserver.ScopeWrite, webserver.ScopeAdmin, webserver.ScopeControl} {
		if webserver.ScopesCover([]string{webserver.ScopeHealthRead}, requested) {
			t.Errorf("health_read unexpectedly covers %q", requested)
		}
	}
}

func TestInvalidScopeHelpListsEverySupportedScope(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/keys", `{"label":"bad","scopes":["unknown"]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid scope status = %d; body=%s", resp.StatusCode, out)
	}
	if want := strings.Join(Scopes, ", "); !strings.Contains(out, want) {
		t.Fatalf("invalid scope help = %s; want supported scopes %q", out, want)
	}
}
