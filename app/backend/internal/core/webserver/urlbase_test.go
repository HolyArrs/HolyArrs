package webserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{"": "", "/": "", "holyarrs": "/holyarrs", "/holyarrs/": "/holyarrs", "/apps/holy-arrs_2": "/apps/holy-arrs_2"} {
		if got, err := CleanURLBase(in); err != nil || got != want {
			t.Errorf("CleanURLBase(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"/a/../b", "//evil.example", "/a b", "/a?x", "/a%2f", "/a//b", "/./x"} {
		if _, err := CleanURLBase(bad); err == nil {
			t.Errorf("CleanURLBase(%q) accepted", bad)
		}
	}
}

func TestHandler(t *testing.T) {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(URLBaseFrom(r.Context()) + " " + r.URL.Path))
	})
	inner.HandleFunc("GET /go", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "s", Value: "v", Path: "/"})
		http.Redirect(w, r, r.URL.Query().Get("to"), http.StatusSeeOther)
	})
	h := UnderURLBase("/holyarrs", inner)
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
		_, _ = w.Write([]byte(URLBaseFrom(r.Context())))
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
	if UnderURLBase("", inner) != http.Handler(inner) {
		t.Fatal("no base should not wrap")
	}
}

// The health check always calls the root, so /healthz and /readyz answer
// there too when a base is set (by flag, env or Settings). Only exact GET
// and HEAD of those two paths; everything else outside the base stays 404.
func TestHandlerHealthAtRootUnderBase(t *testing.T) {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok " + r.URL.Path)) })
	inner.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ready " + r.URL.Path)) })
	inner.HandleFunc("POST /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("posted")) })
	inner.HandleFunc("GET /healthz/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("slash")) })
	h := UnderURLBase("/holyarrs", inner)
	do := func(method, p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, p, nil))
		return w
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/healthz"}, {"HEAD", "/healthz"}, {"GET", "/readyz"}, {"HEAD", "/readyz"},
		{"GET", "/holyarrs/healthz"}, {"GET", "/holyarrs/readyz"},
	} {
		if w := do(c.method, c.path); w.Code != 200 {
			t.Errorf("%s %s under a base: %d, want 200", c.method, c.path, w.Code)
		}
	}
	if w := do("GET", "/healthz"); w.Body.String() != "ok /healthz" {
		t.Errorf("root healthz served %q", w.Body.String())
	}
	if w := do("GET", "/readyz"); w.Body.String() != "ready /readyz" {
		t.Errorf("root readyz served %q", w.Body.String())
	}
	// Each phase keeps its own answer: a failing health stays failing at the root.
	failing := UnderURLBase("/holyarrs", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	w := httptest.NewRecorder()
	failing.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 503 {
		t.Errorf("a failing phase answered %d at the root, want its own 503", w.Code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/healthz"}, {"PUT", "/readyz"}, {"GET", "/healthz/"}, {"GET", "/healthzx"},
		{"GET", "/readyzx"}, {"GET", "/healthz/.."}, {"GET", "/x/../healthz"}, {"GET", "/api/healthz"},
	} {
		if w := do(c.method, c.path); w.Code != 404 {
			t.Errorf("%s %s outside the base: %d, want 404", c.method, c.path, w.Code)
		}
	}
}
