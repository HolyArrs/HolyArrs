package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/ai"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// aiFix is a signed-in app with an owner, a member and their keys.
type aiFix struct {
	*arrCtlFix
	t                      *testing.T
	own, mem               *http.Client
	csrf, memCSRF          string
	ownKey, memKey, allKey string
}

func (f *aiFix) call(c *http.Client, csrf, key, method, path, body string, hdr ...string) (int, string) {
	f.t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(accounts.CSRFHeader, csrf)
	}
	if key != "" {
		req.Header.Set(accounts.APIKeyHeader, key)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func aiField(body, name string) string {
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	s, _ := v[name].(string)
	return s
}

func newAIFix(t *testing.T) *aiFix {
	t.Helper()
	f := &aiFix{arrCtlFix: newArrCtl(t, true), t: t}
	client := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	f.own, f.mem = client(), client()
	st, body := f.call(f.own, "", "", "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	f.csrf = aiField(body, "csrf")
	st, body = f.call(f.own, f.csrf, "", "POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, body)
	}
	st, body = f.call(f.mem, "", "", "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(aiField(body, "path"), "/invite/"), `{"name":"ana","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("join %d %s", st, body)
	}
	f.memCSRF = aiField(body, "csrf")
	// Making keys needs the settings permission.
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","settings"]' WHERE name = 'ana'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mk := func(c *http.Client, csrf, scopes string) string {
		st, body := f.call(c, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"claude","scopes":`+scopes+`}`)
		if st != 201 {
			t.Fatalf("key %s: %d %s", scopes, st, body)
		}
		return aiField(body, "key")
	}
	f.ownKey = mk(f.own, f.csrf, `["mcp"]`)
	f.memKey = mk(f.mem, f.memCSRF, `["mcp"]`)
	f.allKey = mk(f.own, f.csrf, `["all"]`)
	return f
}

func (f *aiFix) turnOn(on bool) {
	f.t.Helper()
	b, _ := json.Marshal(map[string]bool{"enabled": on})
	if st, body := f.call(f.own, f.csrf, "", "PUT", "/api/v1/settings/ai", string(b)); st != 200 {
		f.t.Fatalf("turn on: %d %s", st, body)
	}
}

// rpc sends one JSON-RPC request with key and returns the status and answer.
func (f *aiFix) rpc(key, method, params string) (int, map[string]any) {
	f.t.Helper()
	if params == "" {
		params = "{}"
	}
	st, body := f.call(&http.Client{}, "", key, "POST", "/api/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`)
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	return st, v
}

// tool calls a tool and returns its structured answer, or its error text.
func (f *aiFix) tool(key, name, args string) (map[string]any, string) {
	f.t.Helper()
	st, v := f.rpc(key, "tools/call", `{"name":"`+name+`","arguments":`+args+`}`)
	if st != 200 {
		f.t.Fatalf("%s: %d %v", name, st, v)
	}
	res, _ := v["result"].(map[string]any)
	if res["isError"] == true {
		c, _ := res["content"].([]any)
		first, _ := c[0].(map[string]any)
		s, _ := first["text"].(string)
		return nil, s
	}
	m, _ := res["structuredContent"].(map[string]any)
	return m, ""
}

func (f *aiFix) toolNames(key string) []string {
	f.t.Helper()
	_, v := f.rpc(key, "tools/list", "")
	res, _ := v["result"].(map[string]any)
	ts, _ := res["tools"].([]any)
	var out []string
	for _, t := range ts {
		m, _ := t.(map[string]any)
		out = append(out, m["name"].(string))
	}
	return out
}

// AI connections are off by default (404), an AI key reaches only the AI
// route, other keys and sessions never reach it, and only the owner turns it on.
func TestMCPOffAndScopes(t *testing.T) {
	t.Parallel()
	f := newAIFix(t)
	if st, _ := f.rpc(f.ownKey, "ping", ""); st != 404 {
		t.Fatalf("off: %d", st)
	}
	if st, _ := f.call(f.mem, f.memCSRF, "", "PUT", "/api/v1/settings/ai", `{"enabled":true}`); st != 403 {
		t.Fatalf("member turned it on: %d", st)
	}
	f.turnOn(true)
	if st, v := f.rpc(f.ownKey, "ping", ""); st != 200 || v["result"] == nil {
		t.Fatalf("on: %d %v", st, v)
	}
	if st, _ := f.call(&http.Client{}, "", f.ownKey, "GET", "/api/v1/titles", ""); st != 403 {
		t.Fatalf("AI key on titles: %d", st)
	}
	if st, _ := f.call(&http.Client{}, "", f.ownKey, "POST", "/api/v1/auth/keys", `{"label":"x","scopes":["all"]}`); st != 403 {
		t.Fatalf("AI key made a key: %d", st)
	}
	if st, _ := f.rpc(f.allKey, "ping", ""); st != 403 {
		t.Fatalf("all key on the AI route: %d", st)
	}
	if st, _ := f.call(f.own, f.csrf, "", "POST", "/api/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); st != 401 {
		// A browser session is no AI sign-in: the app is told to sign in.
		t.Fatalf("session on the AI route: %d", st)
	}
	if st, body := f.call(f.own, f.csrf, "", "POST", "/api/v1/auth/keys", `{"label":"mix","scopes":["mcp","read"]}`); st != 400 {
		t.Fatalf("mixed AI key: %d %s", st, body)
	}
	// A bearer key works on the AI route, and on /mcp too.
	st, body := f.call(&http.Client{}, "", "", "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "Authorization", "Bearer "+f.ownKey)
	if st != 200 {
		t.Fatalf("bearer on /mcp: %d %s", st, body)
	}
	if st, _ := f.call(&http.Client{}, "", "", "GET", "/api/v1/titles", "", "Authorization", "Bearer "+f.ownKey); st == 200 {
		t.Fatal("bearer key reached titles")
	}
	f.turnOn(false)
	if st, _ := f.rpc(f.ownKey, "ping", ""); st != 404 {
		t.Fatalf("off again: %d", st)
	}
}

func TestMCPHandshake(t *testing.T) {
	t.Parallel()
	f := newAIFix(t)
	f.turnOn(true)
	st, v := f.rpc(f.ownKey, "initialize", `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"Claude Code","version":"2"}}`)
	res, _ := v["result"].(map[string]any)
	if st != 200 || res["protocolVersion"] != "2025-11-25" || res["capabilities"] == nil {
		t.Fatalf("initialize: %d %v", st, v)
	}
	_, v = f.rpc(f.ownKey, "initialize", `{"protocolVersion":"1999-01-01"}`)
	if res, _ := v["result"].(map[string]any); res["protocolVersion"] != ai.Versions[0] {
		t.Fatalf("unknown version: %v", v)
	}
	_, v = f.rpc(f.ownKey, "server/discover", "")
	if res, _ := v["result"].(map[string]any); res["supportedVersions"] == nil {
		t.Fatalf("discover: %v", v)
	}
	if st, _ := f.call(&http.Client{}, "", f.ownKey, "POST", "/api/v1/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); st != 202 {
		t.Fatalf("notification: %d", st)
	}
	if _, v := f.rpc(f.ownKey, "nope", ""); v["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatalf("unknown method: %v", v)
	}
	if st, _ := f.call(&http.Client{}, "", f.ownKey, "POST", "/api/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "MCP-Protocol-Version", "2020-01-01"); st != 400 {
		t.Fatalf("old protocol header: %d", st)
	}
	if st, _ := f.call(&http.Client{}, "", f.ownKey, "POST", "/api/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, "Origin", "https://evil.example"); st != 403 {
		t.Fatalf("other origin: %d", st)
	}
	if st, _ := f.call(&http.Client{}, "", f.ownKey, "GET", "/api/v1/mcp", ""); st != 405 {
		t.Fatalf("GET: %d", st)
	}
}

// Tools follow the parts and the person's permissions.
func TestMCPToolsFollowPartsAndPerms(t *testing.T) {
	t.Parallel()
	f := newAIFix(t)
	f.turnOn(true)
	has := func(names []string, n string) bool {
		return strings.Contains(","+strings.Join(names, ",")+",", ","+n+",")
	}
	own := f.toolNames(f.ownKey)
	for _, n := range []string{"search_titles", "calendar", "downloads", "my_asks", "health", "ask_for_title"} {
		if !has(own, n) {
			t.Fatalf("owner misses %s: %v", n, own)
		}
	}
	mem := f.toolNames(f.memKey)
	if has(mem, "health") || !has(mem, "ask_for_title") {
		t.Fatalf("member tools: %v", mem)
	}
	// No AI tool approves or turns down someone's request: that stays a
	// person's click on the Requests page.
	for _, n := range []string{"approve_ask", "turn_down_ask"} {
		if has(own, n) {
			t.Fatalf("owner has %s: %v", n, own)
		}
		if _, msg := f.tool(f.ownKey, n, `{"ask_id":1}`); !strings.Contains(msg, "no tool called") {
			t.Fatalf("%s answered: %q", n, msg)
		}
	}
	if err := f.pc.Set(context.Background(), parts.Wishlist, false, domain.System); err != nil {
		t.Fatal(err)
	}
	own = f.toolNames(f.ownKey)
	if has(own, "my_asks") || has(own, "ask_for_title") || !has(own, "calendar") {
		t.Fatalf("Requests off: %v", own)
	}
	if _, msg := f.tool(f.ownKey, "ask_for_title", `{"tmdb_id":"693134"}`); !strings.Contains(msg, "Requests is off") {
		t.Fatalf("hidden tool: %q", msg)
	}
}

// An action takes a confirm token: single use, five minutes, only for
// the key and person it was given to; the action lands in History.
func TestMCPConfirmAndHistory(t *testing.T) {
	t.Parallel()
	f := newAIFix(t)
	f.turnOn(true)
	_, _ = f.rpc(f.memKey, "initialize", `{"protocolVersion":"2026-07-28","clientInfo":{"name":"Claude Desktop"}}`)
	first, msg := f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"693134"}`)
	tok, _ := first["confirm_token"].(string)
	if msg != "" || tok == "" || first["name"] != "Dune: Part Two" {
		t.Fatalf("first step: %v %q", first, msg)
	}
	if n := aiEvents(t, f.s); len(n) != 0 {
		t.Fatalf("first step acted: %v", n)
	}
	// Another person's key cannot use it.
	if _, msg := f.tool(f.ownKey, "ask_for_title", `{"tmdb_id":"693134","confirm_token":"`+tok+`"}`); !strings.Contains(msg, "unknown") {
		t.Fatalf("other user: %q", msg)
	}
	// Nor can it be used for another title.
	if _, msg := f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"1","confirm_token":"`+tok+`"}`); !strings.Contains(msg, "something else") {
		t.Fatalf("other title: %q", msg)
	}
	done, msg := f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"693134","confirm_token":"`+tok+`"}`)
	if msg != "" || done["done"] != true {
		t.Fatalf("second step: %v %q", done, msg)
	}
	if _, msg := f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"693134","confirm_token":"`+tok+`"}`); !strings.Contains(msg, "unknown") {
		t.Fatalf("reuse: %q", msg)
	}
	es := aiEvents(t, f.s)
	if len(es) != 1 || es[0].Kind != "ai_ask_made" || es[0].Actor.Kind != "ai" || es[0].Data["tool"] != "ask_for_title" ||
		es[0].Data["client"] != "Claude Desktop" || es[0].Data["key"] != "claude" || !strings.Contains(es[0].Summary, "Dune") {
		t.Fatalf("event: %+v", es)
	}
	if args, _ := es[0].Data["args"].(map[string]any); args["tmdb_id"] != "693134" || args["confirm_token"] != nil {
		t.Fatalf("logged args: %v", es[0].Data["args"])
	}

	// Expired after five minutes.
	first, _ = f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"693134","kind":"movie"}`)
	tok, _ = first["confirm_token"].(string)
	f.api.AIHandlers().SetClock(func() time.Time { return time.Now().Add(6 * time.Minute) })
	if _, msg := f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"693134","kind":"movie","confirm_token":"`+tok+`"}`); !strings.Contains(msg, "expired") {
		t.Fatalf("expired: %q", msg)
	}
}

func aiEvents(t *testing.T, s *database.Store) []domain.Event {
	t.Helper()
	es, _, err := s.Events(context.Background(), database.EventFilter{Kinds: []string{"ai_*"}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestMCPRateLimit(t *testing.T) {
	t.Parallel()
	f := newAIFix(t)
	f.turnOn(true)
	got := 0
	for i := 0; i < ai.ReadsPerMin+5; i++ {
		st, _ := f.rpc(f.ownKey, "tools/call", `{"name":"downloads","arguments":{}}`)
		if st == 429 {
			got = i
			break
		}
	}
	if got != ai.ReadsPerMin {
		t.Fatalf("429 after %d reads", got)
	}
	// Another key has its own count.
	if st, _ := f.rpc(f.memKey, "tools/call", `{"name":"downloads","arguments":{}}`); st != 200 {
		t.Fatalf("other key: %d", st)
	}
	// Actions: five an hour.
	var s ai.Limits
	for i := 0; i < ai.ActsPerHour; i++ {
		if ok, _ := s.Allow(&s.Acts, 1, ai.ActsPerHour, time.Hour); !ok {
			t.Fatalf("action %d refused", i)
		}
	}
	if ok, wait := s.Allow(&s.Acts, 1, ai.ActsPerHour, time.Hour); ok || wait <= 0 {
		t.Fatal("sixth action allowed")
	}
}

func TestMCPTidy(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 400)
	got := ai.Tidy(map[string]any{"name": "Du​ne\x07", "overview": long, "poster": "x.jpg", "items": make([]any, 40)}, 0).(map[string]any)
	if got["name"] != "Dune" || got["poster"] != nil || len([]rune(got["overview"].(string))) != ai.TextMax+1 {
		t.Fatalf("tidy: %v", got)
	}
	if n := len(got["items"].([]any)); n != 0 {
		t.Fatalf("nil items kept: %d", n)
	}
}
