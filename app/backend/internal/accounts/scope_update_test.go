package accounts

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestUpdateRoutesScopes(t *testing.T) {
	var feedCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/app/update", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"feed":""}`))
	})
	mux.HandleFunc("POST /api/v1/app/update/check", func(w http.ResponseWriter, _ *http.Request) {
		feedCalls.Add(1)
		_, _ = w.Write([]byte(`{}`))
	})
	gate := Gate(mux)
	who := webserver.Caller{UserID: 2, Role: webserver.RoleAdmin, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/v1/app/update", http.StatusOK},
		{http.MethodPost, "/api/v1/app/update/check", http.StatusForbidden},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req = req.WithContext(webserver.ContextWith(req.Context(), who))
		rec := httptest.NewRecorder()
		gate.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s = %d, want %d; body=%s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
	if feedCalls.Load() != 0 {
		t.Fatalf("check reached the feed %d times", feedCalls.Load())
	}
}
