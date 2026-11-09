package webserver

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(t *testing.T, p string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
	return w
}

func TestServesBuiltFilesAndFallsBack(t *testing.T) {
	root := get(t, "/")
	if root.Code != 200 || !strings.Contains(root.Body.String(), "/_app/immutable/") {
		t.Fatalf("/: %d", root.Code)
	}
	for _, p := range []string{"/library", "/settings/sign-in", "/library/12"} {
		if w := get(t, p); w.Code != 200 || w.Body.String() != root.Body.String() {
			t.Errorf("%s: %d, want the app page", p, w.Code)
		}
	}
	for _, p := range []string{"/missing.css", "/_app/immutable/nope.js", "/api/v1/nope", "/live"} {
		if w := get(t, p); w.Code != 404 {
			t.Errorf("%s: %d, want 404", p, w.Code)
		}
	}
	var asset string
	_ = fs.WalkDir(Files, "_app/immutable", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && asset == "" {
			asset = "/" + p
		}
		return nil
	})
	if asset == "" {
		t.Fatal("no built assets; run npm run build in web/app")
	}
	if w := get(t, asset); w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("%s: %d %q", asset, w.Code, w.Header().Get("Cache-Control"))
	}
	if w := get(t, "/"); w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("page cache: %q", w.Header().Get("Cache-Control"))
	}
}

// TestNoBannedWords keeps upstream coined words out of everything the app ships.
func TestNoBannedWords(t *testing.T) {
	banned := []string{"Monitored", "Activity", "Wanted", "Cutoff", "Quality Profile", "Root Folder", "Download Client",
		"Interactive Search", "Manual Import", "Grabbed", "Connections", "Discover", "Collections"}
	n := 0
	_ = fs.WalkDir(Files, ".", func(p string, d fs.DirEntry, err error) error {
		// vendor: third-party code (hls.js), not the app's words (as check-words.mjs).
		if err == nil && d.IsDir() && p == "vendor" {
			return fs.SkipDir
		}
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".js")) {
			return nil
		}
		b, _ := fs.ReadFile(Files, p)
		n++
		for _, w := range banned {
			if strings.Contains(string(b), w) {
				t.Errorf("%s uses the word %q", p, w)
			}
		}
		return nil
	})
	if n == 0 {
		t.Fatal("no app files checked")
	}
}

func TestServesUnderAURLBase(t *testing.T) {
	h := UnderURLBase("/holyarrs", Handler())
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		return w
	}
	page := get("/holyarrs/library/12").Body.String()
	if !strings.Contains(page, `"/holyarrs/_app/immutable/`) || strings.Contains(page, `"/_app/`) || !strings.Contains(page, `base: "/holyarrs"`) ||
		!strings.Contains(page, `href="/holyarrs/brand/`) {
		t.Fatalf("page links not under the base:\n%s", page)
	}
	var css string
	_ = fs.WalkDir(Files, "_app/immutable/assets", func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".css") {
			css = "/" + p
		}
		return nil
	})
	w := get("/holyarrs" + css)
	if w.Code != 200 || strings.Contains(w.Body.String(), "url(/_app/") || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("css %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	// Split scripts list the chunks they preload from the root.
	var js string
	_ = fs.WalkDir(Files, "_app/immutable/entry", func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".js") {
			if b, _ := fs.ReadFile(Files, p); strings.Contains(string(b), `"/_app/`) {
				js = "/" + p
			}
		}
		return nil
	})
	if js != "" {
		w := get("/holyarrs" + js)
		if w.Code != 200 || strings.Contains(w.Body.String(), `"/_app/`) || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
			t.Fatalf("script %s %d %s", js, w.Code, w.Header().Get("Content-Type"))
		}
	}
	if get("/library").Code != 404 {
		t.Fatal("outside the base served")
	}
}

func TestServeBrotli(t *testing.T) {
	fsys := fstest.MapFS{"a.js": {Data: []byte("plain")}, "a.js.br": {Data: []byte("BR")}, "b.css": {Data: []byte("x")}}
	try := func(name, enc string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		r.Header.Set("Accept-Encoding", enc)
		if !ServeBrotli(w, r, fsys, name) {
			return nil
		}
		return w
	}
	w := try("a.js", "gzip, deflate, br")
	if w == nil || w.Body.String() != "BR" || w.Header().Get("Content-Encoding") != "br" || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("brotli copy not served: %+v", w)
	}
	if try("a.js", "gzip") != nil || try("b.css", "br") != nil {
		t.Fatal("served brotli where it should not")
	}
}
