package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

type rig struct {
	h    *Handlers
	mux  *http.ServeMux
	kill func(plugin string) // ends a plugin's token
}

func newRig(t *testing.T, grants map[string][]string) *rig {
	b := newTestBroker(grants)
	h := &Handlers{Reg: NewRegistry(b), ValidScope: func(s string) bool { return strings.Contains(s, ".") }}
	alive := map[string]context.CancelFunc{}
	ctxs := map[string]context.Context{}
	h.Reg.Alive = func(p string) context.Context {
		if _, ok := grants[p]; !ok {
			return nil
		}
		if ctxs[p] == nil {
			ctxs[p], alive[p] = context.WithCancel(context.Background())
		}
		return ctxs[p]
	}
	m := http.NewServeMux()
	h.Routes(m)
	return &rig{h: h, mux: m, kill: func(p string) {
		if c := alive[p]; c != nil {
			c()
		}
		delete(grants, p)
	}}
}

func (g *rig) do(who *webserver.Caller, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if who != nil {
		r = r.WithContext(webserver.ContextWith(r.Context(), *who))
	}
	w := httptest.NewRecorder()
	g.mux.ServeHTTP(w, r)
	return w
}

// asViewer marks r as coming from the signed-in owner's browser session.
func asViewer(r *http.Request) *http.Request {
	return r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{Via: "session", UserID: 1, Role: "owner"}))
}

func plug(id string) *webserver.Caller { return &webserver.Caller{Via: "plugin", PluginID: id} }

var owner = &webserver.Caller{Via: "session", UserID: 1, Role: "owner"}

func TestPluginSideRoutesRefuseEveryoneElse(t *testing.T) {
	g := newRig(t, nil)
	for _, c := range []struct{ m, p string }{
		{"GET", "/api/v1/plugin-hooks/downloader"}, {"POST", "/api/v1/plugin-hooks/releases"},
		{"PUT", "/api/v1/plugin-hooks/tools"}, {"GET", "/api/v1/plugin-hooks/events"},
		{"PUT", "/api/v1/plugin-hooks/home-widget"}, {"PUT", "/api/v1/plugin-hooks/pages"},
		{"GET", "/api/v1/plugin-hooks/users"}, {"POST", "/api/v1/plugin-hooks/stream"},
	} {
		for _, who := range []*webserver.Caller{nil, owner, {Via: "api_key", KeyID: 3, UserID: 1}} {
			if w := g.do(who, c.m, c.p, `{}`); w.Code != http.StatusForbidden {
				t.Errorf("%s %s as %v: %d", c.m, c.p, who, w.Code)
			}
		}
	}
}

func TestWidgetDeclareAndShowTable(t *testing.T) {
	g := newRig(t, map[string][]string{"trakt": {"pages.home-widget"}, "other": {"library.read"}})
	for _, c := range []struct {
		who  string
		body string
		code int
	}{
		{"trakt", `{"title":"Watching","rows":[{"text":"Dune","title_id":3}]}`, http.StatusNoContent},
		{"trakt", `{"title":"<script>","rows":[]}`, http.StatusBadRequest},
		{"other", `{"title":"Hi","rows":[]}`, http.StatusForbidden},
	} {
		if w := g.do(plug(c.who), "PUT", "/api/v1/plugin-hooks/home-widget", c.body); w.Code != c.code {
			t.Errorf("%s %s: %d %s", c.who, c.body, w.Code, w.Body)
		}
	}
	w := g.do(owner, "GET", "/api/v1/plugin-widgets", "")
	var out struct{ Widgets []Widget }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Widgets) != 1 || out.Widgets[0].Plugin != "trakt" || out.Widgets[0].Title != "Watching" {
		t.Fatalf("widgets %s", w.Body)
	}
	g.h.Reg.Drop("trakt")
	w = g.do(owner, "GET", "/api/v1/plugin-widgets", "")
	if strings.Contains(w.Body.String(), "Watching") {
		t.Error("a stopped plugin's widget still shows")
	}
}

func TestUsersNamesOnly(t *testing.T) {
	g := newRig(t, map[string][]string{"bot": {"requests.users.read"}})
	g.h.Users = func(context.Context) ([]User, error) { return []User{{ID: 2, Name: "Ann"}}, nil }
	w := g.do(plug("bot"), "GET", "/api/v1/plugin-hooks/users", "")
	if w.Code != 200 || w.Body.String() != "{\"users\":[{\"id\":2,\"name\":\"Ann\"}]}\n" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestStreamLinkTable(t *testing.T) {
	dir := t.TempDir()
	movie := filepath.Join(dir, "dune.mkv")
	if err := os.WriteFile(movie, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := newRig(t, map[string][]string{"player": {"media.stream"}})
	g.h.StreamFile = func(_ context.Context, pl string, id int64) (string, error) {
		if pl == "player" && id == 7 {
			return movie, nil
		}
		return "", errors.New("this file is not in a folder the plugin was given")
	}
	for _, c := range []struct {
		body string
		code int
	}{
		{`{"file_id":7}`, 200},
		{`{"file_id":8}`, http.StatusForbidden},
		{`{"path":"/etc/passwd"}`, http.StatusBadRequest},
	} {
		if w := g.do(plug("player"), "POST", "/api/v1/plugin-hooks/stream", c.body); w.Code != c.code {
			t.Errorf("%s: %d %s", c.body, w.Code, w.Body)
		}
	}
	w := g.do(plug("player"), "POST", "/api/v1/plugin-hooks/stream", `{"file_id":7}`)
	var out struct{ URL string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	r := asViewer(httptest.NewRequest("GET", out.URL, nil))
	r.Header.Set("Range", "bytes=2-4")
	rec := httptest.NewRecorder()
	g.mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "234" {
		t.Fatalf("range: %d %q", rec.Code, rec.Body)
	}
	g.h.Reg.Drop("player")
	rec = httptest.NewRecorder()
	g.mux.ServeHTTP(rec, asViewer(httptest.NewRequest("GET", out.URL, nil)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a stopped plugin's stream still plays: %d", rec.Code)
	}
}

func TestPluginPageSandboxedAndBridged(t *testing.T) {
	g := newRig(t, map[string][]string{"player": {"pages.add", "library.read"}})
	stub(t, g.h.Reg.Broker, "player", Pages, func(j Job) Answer {
		raw, _ := json.Marshal(FeedAnswer{Status: 200, Body: []byte("<script>parent.postMessage({path:'/api/v1/titles'},'*')</script>")})
		return Answer{Data: raw}
	})
	if w := g.do(plug("player"), "PUT", "/api/v1/plugin-hooks/pages", `[{"id":"player","title":"Player","file":"index.html"}]`); w.Code != http.StatusNoContent {
		t.Fatal(w.Code, w.Body)
	}
	w := g.do(owner, "POST", "/api/v1/plugin-pages/player/player/open", "")
	var out struct{ URL string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	rec := httptest.NewRecorder()
	g.mux.ServeHTTP(rec, httptest.NewRequest("GET", out.URL, nil))
	csp := rec.Header().Get("Content-Security-Policy")
	if rec.Code != 200 || !strings.HasPrefix(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") || !strings.Contains(csp, "connect-src 'none'") {
		t.Fatalf("page file: %d csp %q", rec.Code, csp)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("type %q", ct)
	}
	for _, bad := range []string{"/api/v1/plugin-page-files/nope/index.html", strings.Replace(out.URL, "index.html", "../x.html", 1)} {
		rec = httptest.NewRecorder()
		g.mux.ServeHTTP(rec, httptest.NewRequest("GET", bad, nil))
		if rec.Code == 200 {
			t.Errorf("%s served", bad)
		}
	}
	g.h.BridgeAllowed = func(pl string, who webserver.Caller, r *http.Request) error {
		// plugin holds library.read; only the owner holds the route's permission here
		if r.RemoteAddr == "127.0.0.1:0" || r.URL.Path != "/api/v1/titles" || who.Role != "owner" {
			return errors.New("not allowed")
		}
		return nil
	}
	var ran []string
	g.h.AsPlugin = func(pl string, r *http.Request) (*http.Response, error) {
		ran = append(ran, pl+" "+r.Method+" "+r.URL.Path)
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"error":"no"}`))}, nil
	}
	for _, c := range []struct {
		who  *webserver.Caller
		body string
		code int
	}{
		{owner, `{"method":"GET","path":"/api/v1/titles"}`, 200},
		{owner, `{"method":"GET","path":"/api/v1/settings"}`, 403},
		{&webserver.Caller{Via: "session", UserID: 5, Role: "member"}, `{"method":"GET","path":"/api/v1/titles"}`, 403},
		{owner, `{"method":"GET","path":"/api/v1/../settings"}`, 400},
		{owner, `{"method":"PATCH","path":"/api/v1/titles"}`, 400},
		{owner, `{"method":"GET","path":"https://evil/"}`, 400},
		{plug("player"), `{"method":"GET","path":"/api/v1/titles"}`, 403},
		{&webserver.Caller{Via: "api_key", KeyID: 2, UserID: 1}, `{"method":"GET","path":"/api/v1/titles"}`, 403},
	} {
		if w := g.do(c.who, "POST", "/api/v1/plugin-pages/player/call", c.body); w.Code != c.code {
			t.Errorf("%v %s: %d %s", c.who.Via, c.body, w.Code, w.Body)
		}
	}
	if len(ran) != 1 || ran[0] != "player GET /api/v1/titles" {
		t.Errorf("bridge ran %v", ran)
	}
}

func TestEveryHookHandlerChecksItsOwnScope(t *testing.T) {
	g := newRig(t, map[string][]string{"none": {"library.read"}})
	g.h.Users = func(context.Context) ([]User, error) { return []User{{ID: 1, Name: "A"}}, nil }
	g.h.StreamFile = func(context.Context, string, int64) (string, error) { return "x", nil }
	for _, c := range []struct{ m, p, body string }{
		{"GET", "/api/v1/plugin-hooks/users", ""},
		{"POST", "/api/v1/plugin-hooks/stream", `{"file_id":1}`},
		{"PUT", "/api/v1/plugin-hooks/tools", `[]`},
		{"PUT", "/api/v1/plugin-hooks/pages", `[]`},
		{"PUT", "/api/v1/plugin-hooks/home-widget", `{"title":"x","rows":[]}`},
		{"GET", "/api/v1/plugin-hooks/downloader", ""},
		{"POST", "/api/v1/plugin-hooks/releases", `{"job":"x"}`},
		{"GET", "/api/v1/plugin-hooks/events", ""},
		{"POST", "/api/v1/plugin-hooks/sections", `{"job":"x"}`},
	} {
		if w := g.do(plug("none"), c.m, c.p, c.body); w.Code != http.StatusForbidden {
			t.Errorf("%s %s without its scope: %d %s", c.m, c.p, w.Code, w.Body)
		}
	}
}

func TestStreamRecheckedOnEveryRequest(t *testing.T) {
	dir := t.TempDir()
	movie := filepath.Join(dir, "dune.mkv")
	if err := os.WriteFile(movie, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	grants := map[string][]string{"player": {"media.stream"}}
	g := newRig(t, grants)
	inFolder := true
	g.h.StreamFile = func(context.Context, string, int64) (string, error) {
		if !inFolder {
			return "", errors.New("not in a granted folder")
		}
		return movie, nil
	}
	w := g.do(plug("player"), "POST", "/api/v1/plugin-hooks/stream", `{"file_id":7}`)
	var out struct{ URL string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if strings.Contains(out.URL, "dune") {
		t.Fatalf("link names the file: %s", out.URL)
	}
	get := func() int {
		rec := httptest.NewRecorder()
		g.mux.ServeHTTP(rec, asViewer(httptest.NewRequest("GET", out.URL, nil)))
		return rec.Code
	}
	for _, c := range []struct {
		name  string
		setup func()
		code  int
	}{
		{"granted, in folder", func() {}, 200},
		{"folder taken away", func() { inFolder = false }, 404},
		{"folder back", func() { inFolder = true }, 200},
		{"scope taken away", func() { grants["player"] = []string{"library.read"} }, 404},
	} {
		c.setup()
		if got := get(); got != c.code {
			t.Errorf("%s: %d, want %d", c.name, got, c.code)
		}
	}
}

func TestStreamFetchRulesTable(t *testing.T) {
	dir := t.TempDir()
	movie := filepath.Join(dir, "dune.mkv")
	if err := os.WriteFile(movie, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	g := newRig(t, map[string][]string{"player": {"media.stream"}})
	g.h.StreamFile = func(context.Context, string, int64) (string, error) { return movie, nil }
	link := func() string {
		w := g.do(plug("player"), "POST", "/api/v1/plugin-hooks/stream", `{"file_id":7}`)
		var out struct{ URL string }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out.URL
	}
	for _, c := range []struct {
		name, dest, origin, addr string
		code                     int
	}{
		{"video element in a sandboxed page", "video", "null", "10.0.0.5:1", http.StatusPartialContent},
		{"plain fetch", "", "", "10.0.0.5:1", http.StatusPartialContent},
		{"opened as a page", "document", "", "10.0.0.5:1", http.StatusNotFound},
		{"loaded in a frame", "iframe", "", "10.0.0.5:1", http.StatusNotFound},
		{"script", "script", "", "10.0.0.5:1", http.StatusNotFound},
		{"another site's page", "video", "https://evil.example", "10.0.0.5:1", http.StatusNotFound},
	} {
		r := asViewer(httptest.NewRequest("GET", link(), nil))
		r.RemoteAddr = c.addr
		r.Header.Set("Range", "bytes=0-9")
		if c.dest != "" {
			r.Header.Set("Sec-Fetch-Dest", c.dest)
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		w := httptest.NewRecorder()
		g.mux.ServeHTTP(w, r)
		if w.Code != c.code {
			t.Errorf("%s: %d want %d", c.name, w.Code, c.code)
		}
	}
	u := link()
	for _, c := range []struct {
		name   string
		viewer bool
		header string
		code   int
	}{
		{"signed-in browser", true, "", 206},
		{"no session", false, "", 404},
		{"through a proxy", true, "X-Forwarded-For", 404},
		{"through a forwarding plugin", true, "X-Holyarrs-Plugin-Proxy", 404},
		{"forwarded header", true, "Forwarded", 404},
	} {
		r := httptest.NewRequest("GET", u, nil)
		if c.viewer {
			r = asViewer(r)
		}
		if c.header != "" {
			r.Header.Set(c.header, "x")
		}
		r.Header.Set("Range", "bytes=0-9")
		w := httptest.NewRecorder()
		g.mux.ServeHTTP(w, r)
		if w.Code != c.code {
			t.Errorf("%s: %d", c.name, w.Code)
		}
	}
	if streamLife > 15*time.Minute {
		t.Errorf("stream links live %s", streamLife)
	}
}

func TestRevokeDuringStream(t *testing.T) {
	dir := t.TempDir()
	movie := filepath.Join(dir, "big.mkv")
	if err := os.WriteFile(movie, make([]byte, 64<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	g := newRig(t, map[string][]string{"player": {"media.stream"}})
	g.h.StreamFile = func(context.Context, string, int64) (string, error) { return movie, nil }
	w := g.do(plug("player"), "POST", "/api/v1/plugin-hooks/stream", `{"file_id":7}`)
	var out struct{ URL string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { g.mux.ServeHTTP(w, asViewer(r)) }))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + out.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 64<<10)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	g.kill("player")
	n, _ := io.Copy(io.Discard, resp.Body)
	if n+int64(len(buf)) >= 64<<20 {
		t.Errorf("the whole file streamed after revoke (%d bytes after)", n)
	}
}
