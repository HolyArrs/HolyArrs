package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Bazarr writes its key into its start page when its sign-in is off.
func TestLookupKeyReadsBazarrStartPage(t *testing.T) {
	key := strings.Repeat("ab", 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<html><script>window.Bazarr = {"apiKey": "` + key + `", "baseUrl": ""}</script></html>`))
	}))
	defer srv.Close()
	if got := LookupKey(context.Background(), Bazarr, srv.URL, true); got != key {
		t.Fatalf("bazarr key not found (len %d)", len(got))
	}
}
