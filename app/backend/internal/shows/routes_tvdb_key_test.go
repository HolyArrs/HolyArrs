package shows

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// route: GET /api/v1/settings/tvdb-key
// route: PUT /api/v1/settings/tvdb-key
func TestTVDBKeyAPI(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&TVDBKeyHandlers{Secrets: v}).Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	do := func(method, path, body string, want int) string {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, res.StatusCode, want, b)
		}
		return string(b)
	}
	if b := do("GET", "/api/v1/settings/tvdb-key", "", 200); !strings.Contains(b, `"key":""`) {
		t.Fatalf("no key yet: %s", b)
	}
	do("PUT", "/api/v1/settings/tvdb-key", `{}`, 400)
	do("PUT", "/api/v1/settings/tvdb-key", `{"key":"TVDB_FOR_TEST_ONLY"}`, 200)
	b := do("GET", "/api/v1/settings/tvdb-key", "", 200)
	if strings.Contains(b, "TVDB_FOR_TEST_ONLY") || !strings.Contains(b, secrets.Masked) {
		t.Fatalf("tvdb key shown: %s", b)
	}
	do("PUT", "/api/v1/settings/tvdb-key", `{"key":""}`, 200)
	if b := do("GET", "/api/v1/settings/tvdb-key", "", 200); !strings.Contains(b, `"key":""`) {
		t.Fatalf("key not cleared: %s", b)
	}
}
