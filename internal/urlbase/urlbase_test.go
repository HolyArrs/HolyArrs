package urlbase

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{"": "", "/": "", "holyarrs": "/holyarrs", "/holyarrs/": "/holyarrs", "/apps/holy-arrs_2": "/apps/holy-arrs_2"} {
		if got, err := Clean(in); err != nil || got != want {
			t.Errorf("Clean(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"/a/../b", "//evil.example", "/a b", "/a?x", "/a%2f", "/a//b", "/./x"} {
		if _, err := Clean(bad); err == nil {
			t.Errorf("Clean(%q) accepted", bad)
		}
	}
}

func TestHandler(t *testing.T) {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(From(r.Context()) + " " + r.URL.Path))
	})
	inner.HandleFunc("GET /go", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "s", Value: "v", Path: "/"})
		http.Redirect(w, r, r.URL.Query().Get("to"), http.StatusSeeOther)
	})
	h := Handler("/holyarrs", inner)
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w
	}
	if w := get("/holyarrs/login"); w.Code != 200 || w.Body.String() != "/holyarrs /login" {
		t.Fatalf("served %d %q", w.Code, w.Body.String())
	}
	// AI app sign-in looks for the server's details at the root (RFC 8414).
	inner.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(From(r.Context())))
	})
	if w := get("/.well-known/oauth-authorization-server/holyarrs"); w.Code != 200 || w.Body.String() != "/holyarrs" {
		t.Fatalf("sign-in details at the root: %d %q", w.Code, w.Body.String())
	}
	if w := get("/.well-known/oauth-authorization-server"); w.Code != 404 {
		t.Fatalf("root without the base: %d", w.Code)
	}
	if w := get("/holyarrs"); w.Code != 301 || w.Header().Get("Location") != "/holyarrs/" {
		t.Fatalf("bare base %d %q", w.Code, w.Header().Get("Location"))
	}
	for _, p := range []string{"/login", "/holyarrsx/login", "/"} {
		if w := get(p); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	w := get("/holyarrs/go?to=/library")
	if w.Header().Get("Location") != "/holyarrs/library" || w.Header().Get("Set-Cookie") != "s=v; Path=/holyarrs/" {
		t.Fatalf("fixed %q %q", w.Header().Get("Location"), w.Header().Get("Set-Cookie"))
	}
	// Never turned into an open redirect, never sent elsewhere.
	for to, want := range map[string]string{"//evil.example/x": "//evil.example/x", "https://evil.example/": "https://evil.example/"} {
		if got := get("/holyarrs/go?to=" + to).Header().Get("Location"); got != want {
			t.Errorf("%s became %q", to, got)
		}
	}
	if Handler("", inner) != http.Handler(inner) {
		t.Fatal("no base should not wrap")
	}
}
