package api

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/secrets"
)

// route: GET /api/v1/settings/tvdb-key
// route: PUT /api/v1/settings/tvdb-key
func TestTVDBKeyAPI(t *testing.T) {
	srv, _ := listSetup(t)
	if b := string(do(t, "GET", srv.URL+"/api/v1/settings/tvdb-key", "", 200)); !strings.Contains(b, `"key":""`) {
		t.Fatalf("no key yet: %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/tvdb-key", `{}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/tvdb-key", `{"key":"TVDB_FOR_TEST_ONLY"}`, 200)
	b := string(do(t, "GET", srv.URL+"/api/v1/settings/tvdb-key", "", 200))
	if strings.Contains(b, "TVDB_FOR_TEST_ONLY") || !strings.Contains(b, secrets.Masked) {
		t.Fatalf("tvdb key shown: %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/tvdb-key", `{"key":""}`, 200)
	if b := string(do(t, "GET", srv.URL+"/api/v1/settings/tvdb-key", "", 200)); !strings.Contains(b, `"key":""`) {
		t.Fatalf("key not cleared: %s", b)
	}
}
