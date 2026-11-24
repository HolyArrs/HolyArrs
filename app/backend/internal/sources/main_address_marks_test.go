package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
)

// testAt runs a Source Test against a fake server and returns the words shown.
func testAt(t *testing.T, path string, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	addr := srv.URL + path
	_, err := New(Config{URL: addr, APIKey: "k", AllowLocal: true, Timeout: 5 * time.Second}).Test(context.Background())
	if err == nil {
		t.Fatalf("%s: test passed", path)
	}
	return ExplainVia(addr, err, nil)
}

func page(status int, head map[string]string, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range head {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

// Prowlarr's or Jackett's main page is spotted by its answer on any port, URL base or proxy.
func TestMainAddressSpottedByMarks(t *testing.T) {
	prowlarrUI := page(200, map[string]string{"Content-Type": "text/html"}, "<!doctype html><html><head><title>Prowlarr</title></head></html>")
	jackettUI := page(200, map[string]string{"Content-Type": "text/html"}, "<html><head><title>Jackett</title></head></html>")
	prowlarr401 := page(401, map[string]string{"X-Application-Version": "1.30.0"}, `{"message":"Unauthorized","app":"Prowlarr"}`)
	for _, tc := range []struct {
		path string
		h    http.HandlerFunc
		want string
	}{
		{"/", prowlarrUI, "That is Prowlarr's main address"},
		{"/prowlarr/", prowlarrUI, "That is Prowlarr's main address"},
		{"/prowlarr/api", prowlarr401, "That is Prowlarr's main address"},
		{"/", jackettUI, "That is Jackett's main address"},
		{"/jackett", jackettUI, "That is Jackett's main address"},
		// A page that names no app stays another app.
		{"/", page(200, nil, "<html><head><title>Welcome</title></head></html>"), "Another app answered"},
	} {
		if got := testAt(t, tc.path, tc.h); !strings.HasPrefix(got, tc.want) {
			t.Errorf("%s: got %q, want it to start %q", tc.path, got, tc.want)
		}
	}
}

// A refused key on one site's feed says so, whatever answered.
func TestSiteFeedRefusalIsTheKey(t *testing.T) {
	auth := doctor.Line(doctor.Auth)
	sonarrish := map[string]string{"Content-Type": "text/html"}
	for _, tc := range []struct {
		path string
		h    http.HandlerFunc
	}{
		{"/5/api", page(401, map[string]string{"X-Application-Version": "1.30.0"}, "")},
		{"/5/api", page(403, sonarrish, "<html><title>Prowlarr</title></html>")},
		{"/prowlarr/5/api", page(401, sonarrish, "<html><title>Sonarr</title></html>")},
		{"/5/api", page(401, sonarrish, "<html><title>Sonarr</title></html>")},
		{"/api/v2.0/indexers/nyaa/results/torznab/api", page(401, sonarrish, "<html><title>Jackett</title></html>")},
	} {
		if got := testAt(t, tc.path, tc.h); got != auth {
			t.Errorf("%s: got %q, want %q", tc.path, got, auth)
		}
	}
}

// Wrong-app spotting still holds where the path names no site, and NZBHydra stays its own.
func TestWrongAppOffSiteFeeds(t *testing.T) {
	sonarr := page(401, map[string]string{"Content-Type": "text/html"}, "<html><title>Sonarr</title></html>")
	if got := testAt(t, "/api", sonarr); !strings.HasPrefix(got, "Another app answered") {
		t.Errorf("sonarr on /api: %q", got)
	}
	if got := testAt(t, "/5/api", page(404, map[string]string{"Content-Type": "text/html"}, "<html><title>Sonarr</title></html>")); !strings.HasPrefix(got, "Another app answered") {
		t.Errorf("sonarr 404 on /5/api: %q", got)
	}
	hydra := page(401, map[string]string{"Content-Type": "text/html"}, "<html><title>NZBHydra 2</title></html>")
	for _, p := range []string{"/api", "/torznab/api"} {
		if got := testAt(t, p, hydra); got != doctor.Line(doctor.Auth) {
			t.Errorf("nzbhydra %s: %q", p, got)
		}
	}
}
