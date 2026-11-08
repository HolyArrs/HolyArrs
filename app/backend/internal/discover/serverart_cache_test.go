package discover

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// route: GET /api/v1/artwork/img/{kind}/{id}/{file}
// The server's stored title and person pictures come through the artwork
// cache: fetched once, a versioned address kept a year, anything that is
// not a stored picture refused before any request.
func TestServerArtworkThroughTheCache(t *testing.T) {
	t.Parallel()
	var asked atomic.Int32
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		switch r.URL.Path {
		case "/v1/img/title-logo/movie-603/1200.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "\x89PNG logo")
		case "/v1/img/person/287/500.webp":
			w.Header().Set("Content-Type", "image/webp")
			_, _ = io.WriteString(w, "RIFF photo "+r.URL.RawQuery)
		default:
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
		}
	}))
	defer pics.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/v1/img/"}, HTTP: pics.Client()}
	var misses ArtworkBudget
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/artwork/img/{kind}/{id}/{file}", func(w http.ResponseWriter, r *http.Request) { ServerArtwork(w, r, pics.URL+"/v1/img", art, &misses) })
	srv := httptest.NewServer(m)
	defer srv.Close()
	get := func(p string) (int, string, string) {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Cache-Control"), string(b)
	}
	if code, _, body := get("/api/v1/artwork/img/title-logo/movie-603/1200.png"); code != 200 || body != "\x89PNG logo" {
		t.Fatalf("logo %d %q", code, body)
	}
	get("/api/v1/artwork/img/title-logo/movie-603/1200.png")
	if n := asked.Load(); n != 1 {
		t.Fatalf("logo asked %d times, want once", n)
	}
	code, cc, body := get("/api/v1/artwork/img/person/287/500.webp?v=3f")
	if code != 200 || body != "RIFF photo v=3f" || !strings.Contains(cc, "immutable") {
		t.Fatalf("versioned photo %d %q %q", code, cc, body)
	}
	if code, _, _ := get("/api/v1/artwork/img/title-wide/tv-1399/1600.webp"); code != 404 {
		t.Fatalf("a picture the server does not have: %d", code)
	}
	before := asked.Load()
	for _, p := range []string{
		"/api/v1/artwork/img/clearart/movie-603/1000.png",
		"/api/v1/artwork/img/person/abc/500.webp",
		"/api/v1/artwork/img/title-logo/603/1200.png",
		"/api/v1/artwork/img/title-logo/movie-603/logo.svg",
		"/api/v1/artwork/img/person/287/500.webp?v=a%20b",
	} {
		if code, _, _ := get(p); code != 404 {
			t.Errorf("%s: %d, want 404", p, code)
		}
	}
	if asked.Load() != before {
		t.Fatal("an odd address reached the server")
	}
}
