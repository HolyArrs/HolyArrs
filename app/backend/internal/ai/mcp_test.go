package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// rig is a Handlers over a real store and a small stand-in for the app's
// routes. partsOff holds the parts that are switched off.
type rig struct {
	h        *Handlers
	st       *database.Store
	who      webserver.Caller
	partsOff map[parts.Part]bool
	asked    int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := saveEnabled(context.Background(), st, database.System, true); err != nil {
		t.Fatal(err)
	}
	g := &rig{st: st, partsOff: map[parts.Part]bool{}}
	g.who = webserver.Caller{UserID: 7, Name: "Sam", Via: "api_key", Role: webserver.RoleOwner, KeyID: 3, KeyLabel: "test key",
		Scopes: []string{webserver.ScopeMCP}, Perms: webserver.AllPerms}
	g.h = &Handlers{Store: st, Version: "test", PartOn: func(p parts.Part) bool { return !g.partsOff[p] }}
	g.h.Inner = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/asks":
			g.asked++
			if g.asked > 1 {
				webserver.Fail(w, http.StatusConflict, "already asked for")
				return
			}
			webserver.WriteJSON(w, http.StatusCreated, map[string]any{"ask": map[string]any{"id": 11, "name": "Some Movie"}})
		case r.URL.Path == "/api/v1/shows/lookup" || r.URL.Path == "/api/v1/titles/lookup":
			webserver.WriteJSON(w, http.StatusOK, map[string]any{"results": []any{map[string]any{"name": "Looked Up", "year": 2020}}})
		case strings.HasPrefix(r.URL.Path, "/api/v1/metadata/"):
			webserver.Fail(w, http.StatusNotFound, "no details")
		default:
			webserver.WriteJSON(w, http.StatusOK, map[string]any{})
		}
	})
	return g
}

func (g *rig) request() *http.Request {
	r := httptest.NewRequest("POST", "/mcp", nil)
	return r.WithContext(webserver.ContextWith(r.Context(), g.who))
}

// tool runs one tool and gives the structured answer, the text and whether
// it was an error.
func (g *rig) tool(name string, args map[string]any) (map[string]any, string, bool) {
	res, _ := g.h.callTool(g.request(), g.who, name, args)
	isErr, _ := res["isError"].(bool)
	text := ""
	if c, ok := res["content"].([]any); ok && len(c) > 0 {
		text, _ = c[0].(map[string]any)["text"].(string)
	}
	m, _ := res["structuredContent"].(map[string]any)
	return m, text, isErr
}

func (g *rig) toolNames() []string {
	var out []string
	for _, ts := range g.h.tools() {
		if ts.off(g.h, g.who) == "" {
			out = append(out, ts.name)
		}
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func failedEvents(t *testing.T, st *database.Store) []database.Event {
	t.Helper()
	es, _, err := st.Events(context.Background(), database.EventFilter{Kinds: []string{"ai_action_failed"}})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestMCPFailedActionLogged(t *testing.T) {
	g := newRig(t)
	args := func(tok string) map[string]any {
		a := map[string]any{"tmdb_id": "693134"}
		if tok != "" {
			a["confirm_token"] = tok
		}
		return a
	}
	confirmed := func() (map[string]any, string, bool) {
		first, _, isErr := g.tool("ask_for_title", args(""))
		tok, _ := first["confirm_token"].(string)
		if isErr || tok == "" {
			t.Fatalf("first step: %v", first)
		}
		return g.tool("ask_for_title", args(tok))
	}
	if _, msg, isErr := confirmed(); isErr {
		t.Fatalf("first ask failed: %s", msg)
	}
	if n := len(failedEvents(t, g.st)); n != 0 {
		t.Fatalf("failed events after a good ask: %d", n)
	}
	_, msg, isErr := confirmed()
	if !isErr || !strings.Contains(msg, "already asked for") {
		t.Fatalf("second ask: isError=%v msg=%q, want the route's own words", isErr, msg)
	}
	es := failedEvents(t, g.st)
	if len(es) != 1 {
		t.Fatalf("failed events: %d", len(es))
	}
	if es[0].Data["ok"] != false || es[0].Data["error"] != "already asked for" || es[0].Data["tool"] != "ask_for_title" {
		t.Fatalf("event data: %v", es[0].Data)
	}
}

func TestToolCallWithoutInner(t *testing.T) {
	h := &Handlers{}
	who := webserver.Caller{UserID: 1, Via: "api_key", KeyID: 1, Scopes: []string{webserver.ScopeMCP}}
	req := httptest.NewRequest("POST", "/mcp", nil)
	res, wait := h.callTool(req, who, "downloads", map[string]any{})
	if wait != 0 || res["isError"] != true {
		t.Fatalf("result %v wait %v", res, wait)
	}
	text, _ := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "503") {
		t.Fatalf("text %q does not say 503", text)
	}
}

func TestMCPMediaToolsHiddenWhenAllMediaOff(t *testing.T) {
	g := newRig(t)
	media := []string{"search_titles", "library_lookup", "calendar"}
	for _, n := range media {
		if !has(g.toolNames(), n) {
			t.Fatalf("%s missing with every part on: %v", n, g.toolNames())
		}
	}
	g.partsOff[parts.Movies], g.partsOff[parts.Shows], g.partsOff[parts.Music] = true, true, true
	names := g.toolNames()
	for _, n := range media {
		if has(names, n) {
			t.Errorf("%s still shown with Movies, Shows and Music off", n)
		}
	}
	if !has(names, "downloads") {
		t.Errorf("downloads hidden: %v", names)
	}
	g.partsOff[parts.Music] = false
	for _, n := range media {
		if !has(g.toolNames(), n) {
			t.Errorf("%s not back with Music on", n)
		}
	}
}

func TestMCPAskShowFollowsShowsPart(t *testing.T) {
	g := newRig(t)
	g.partsOff[parts.Shows] = true
	m, msg, isErr := g.tool("ask_for_title", map[string]any{"tmdb_id": "1399", "kind": "show"})
	if !isErr || !strings.Contains(msg, "is off") || m["confirm_token"] != nil {
		t.Fatalf("with Shows off: isError=%v msg=%q %v", isErr, msg, m)
	}
	// A movie is still allowed while only Shows is off.
	if m, _, isErr := g.tool("ask_for_title", map[string]any{"tmdb_id": "1399", "kind": "movie"}); isErr || m["confirm_token"] == nil {
		t.Fatalf("movie with only Shows off: %v", m)
	}
	g.partsOff[parts.Shows] = false
	m, msg, isErr = g.tool("ask_for_title", map[string]any{"tmdb_id": "1399", "kind": "show"})
	if isErr || m["confirm_token"] == nil || m["name"] != "Looked Up" {
		t.Fatalf("with Shows on: isError=%v msg=%q %v", isErr, msg, m)
	}
}

func TestMCPProtocolEdges(t *testing.T) {
	g := newRig(t)
	rpc := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(webserver.ContextWith(r.Context(), g.who))
		rec := httptest.NewRecorder()
		g.h.serveRPC(rec, r)
		return rec
	}
	for _, p := range []string{"/api/v1/mcp", "/mcp"} {
		rec := rpc("GET", p, "")
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
			t.Errorf("GET %s: %d Allow=%q", p, rec.Code, rec.Header().Get("Allow"))
		}
	}
	rec := rpc("POST", "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Errorf("notification: %d %q", rec.Code, rec.Body.String())
	}
	code := func(rec *httptest.ResponseRecorder) float64 {
		var v struct {
			Error struct {
				Code float64 `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		return v.Error.Code
	}
	rec = rpc("POST", "/mcp", `{bad`)
	if rec.Code != http.StatusBadRequest || code(rec) != -32600 {
		t.Errorf("bad JSON: %d %s", rec.Code, rec.Body.String())
	}
	rec = rpc("POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"nope/x"}`)
	if rec.Code != http.StatusOK || code(rec) != -32601 {
		t.Errorf("unknown method: %d %s", rec.Code, rec.Body.String())
	}
}
