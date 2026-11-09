package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// With nothing wired in, every control refuses plainly instead of acting,
// and the live interest call wants a known view.
func TestControlsRefuseWhenNotWired(t *testing.T) {
	m := http.NewServeMux()
	h := &ControlHandlers{LinkTest: func(string) (http.HandlerFunc, string) { return nil, "" }}
	h.Routes(m)
	(&Auth{}).LiveRoutes(m, nil, &webserver.Interest{}, nil)
	srv := httptest.NewServer(webserver.RecordRouteHits(m))
	defer srv.Close()
	for _, c := range []struct {
		path string
		want int
	}{
		{"/api/v1/stages/search/pause", http.StatusServiceUnavailable},
		{"/api/v1/stages/search/resume", http.StatusServiceUnavailable},
		{"/api/v1/stages/search/restart", http.StatusServiceUnavailable},
		{"/api/v1/links/nope/1/restart", http.StatusNotFound},
		{"/api/v1/app/restart", http.StatusServiceUnavailable},
		{"/api/v1/app/safe-mode", http.StatusServiceUnavailable},
		{"/api/v1/app/shutdown", http.StatusServiceUnavailable},
		{"/api/v1/live/interest", http.StatusBadRequest},
	} {
		res, err := http.Post(srv.URL+c.path, "application/json", strings.NewReader(`{"view":"other"}`))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != c.want {
			t.Errorf("%s: got %d, want %d", c.path, res.StatusCode, c.want)
		}
	}
}
