package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A TMDB picture asked of us when TMDB's network fails is fetched and
// passed straight through, kept a year at the edge, never stored.
func TestTMDBImagePassThrough(t *testing.T) {
	asked := 0
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.URL.Path != "/w500/abcDEF123.jpg" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpegbytes"))
	}))
	defer tmdb.Close()
	s, _, calls := setup(Options{Burst: 100, TMDBImages: tmdb.URL, TMDBImageClient: tmdb.Client()})
	w := req(s, "/v1/img/tmdb/w500/abcDEF123.jpg", "")
	if w.Code != 200 || w.Body.String() != "jpegbytes" || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("pass-through %d %v", w.Code, w.Header())
	}
	if w := req(s, "/v1/img/tmdb/w500/missing1.jpg", ""); w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing %d %v", w.Code, w.Header())
	}
	for _, bad := range []string{"/v1/img/tmdb/w9999/abcDEF123.jpg", "/v1/img/tmdb/w500/../etc.jpg", "/v1/img/tmdb/w500/abcDEF123.jpg?x=1", "/v1/img/tmdb/w500/abc.exe"} {
		if w := req(s, bad, ""); w.Code != 400 {
			t.Fatalf("%s accepted %d", bad, w.Code)
		}
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") != "" {
		t.Fatalf("jpeg headers %v", w.Header())
	}
	if asked != 2 || calls.Load() != 0 {
		t.Fatalf("TMDB asked %d times, providers %d", asked, calls.Load())
	}
}

// Only pictures pass through; an SVG answers with nosniff and a policy
// that lets nothing in it run when opened on its own.
func TestTMDBImageTypesAndSVGPolicy(t *testing.T) {
	types := map[string]string{"/w500/logoLogo.svg": "image/svg+xml", "/w500/pagePage.jpg": "text/html; charset=utf-8", "/w500/plainText.png": "text/plain", "/w500/webpWebp.webp": "image/webp"}
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", types[r.URL.Path])
		_, _ = w.Write([]byte("<svg><script>alert(1)</script></svg>"))
	}))
	defer tmdb.Close()
	s, _, _ := setup(Options{Burst: 100, TMDBImages: tmdb.URL, TMDBImageClient: tmdb.Client()})
	w := req(s, "/v1/img/tmdb/w500/logoLogo.svg", "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/svg+xml" || w.Header().Get("X-Content-Type-Options") != "nosniff" ||
		w.Header().Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'; sandbox" {
		t.Fatalf("svg %d %v", w.Code, w.Header())
	}
	if w := req(s, "/v1/img/tmdb/w500/webpWebp.webp", ""); w.Code != 200 || w.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("webp %d %v", w.Code, w.Header())
	}
	for _, p := range []string{"/v1/img/tmdb/w500/pagePage.jpg", "/v1/img/tmdb/w500/plainText.png"} {
		if w := req(s, p, ""); w.Code == 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s passed through: %d %v", p, w.Code, w.Header())
		}
	}
}
