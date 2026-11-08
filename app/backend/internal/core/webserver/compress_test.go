package webserver

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccepts(t *testing.T) {
	for in, want := range map[string]bool{"gzip, br": true, "br;q=1, gzip;q=0.5": true, "gzip;q=0": false, "br": false, "": false, "GZIP": true} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Encoding", in)
		if got := AcceptsEncoding(r, "gzip"); got != want {
			t.Errorf("AcceptsEncoding(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCompressible(t *testing.T) {
	for ct, want := range map[string]bool{"application/json": true, "text/html; charset=utf-8": true, "text/javascript": true, "image/svg+xml": true,
		"image/jpeg": false, "font/woff2": false, "text/event-stream": false, "application/zip": false} {
		if got := Compressible(ct); got != want {
			t.Errorf("Compressible(%q) = %v, want %v", ct, got, want)
		}
	}
}

func serve(t *testing.T, h http.HandlerFunc, enc string) *http.Response {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v1/x", nil)
	if enc != "" {
		r.Header.Set("Accept-Encoding", enc)
	}
	rec := httptest.NewRecorder()
	Compress(h).ServeHTTP(rec, r)
	return rec.Result()
}

func TestHandlerGzipsJSON(t *testing.T) {
	body := `{"titles":[` + strings.Repeat(`{"name":"a movie"},`, 200) + `{}]}`
	res := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}, "gzip, br")
	if res.Header.Get("Content-Encoding") != "gzip" || !strings.Contains(res.Header.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("headers %v", res.Header)
	}
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != body {
		t.Fatal("body changed")
	}
}

func TestHandlerLeavesOthersAlone(t *testing.T) {
	big := strings.Repeat("x", 5000)
	cases := map[string]struct {
		h   http.HandlerFunc
		enc string
	}{
		"no accept": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, big)
		}, ""},
		"picture": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = io.WriteString(w, big)
		}, "gzip"},
		"small": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "2")
			_, _ = io.WriteString(w, "{}")
		}, "gzip"},
		"encoded": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/javascript")
			w.Header().Set("Content-Encoding", "br")
			_, _ = io.WriteString(w, big)
		}, "gzip, br"},
		"not modified": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(304)
		}, "gzip"},
	}
	for name, c := range cases {
		res := serve(t, c.h, c.enc)
		if res.Header.Get("Content-Encoding") == "gzip" {
			t.Errorf("%s: gzipped", name)
		}
	}
}
