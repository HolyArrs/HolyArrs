package ai

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// toolSpec is one tool: off says why it is hidden from this person ("" when
// shown), run does the work.
type toolSpec struct {
	name, title, about string
	action             bool
	schema             map[string]any
	off                func(h *Handlers, who webserver.Caller) string
	run                func(h *Handlers, r *http.Request, who webserver.Caller, args map[string]any) (any, error)
}

// failure is a tool error said to the AI in plain words.
type failure struct{ msg string }

// Error returns the message shown to the client.
func (e failure) Error() string { return e.msg }

func object(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	strProp = func(about string) map[string]any { return map[string]any{"type": "string", "description": about} }
	intProp = func(about string) map[string]any { return map[string]any{"type": "integer", "description": about} }
	tokProp = strProp("Leave out on the first call. Send the confirm_token from the first answer after the person agreed.")
)

// Who may see a tool.
func mediaOff(h *Handlers, _ webserver.Caller) string {
	for _, p := range []parts.Part{parts.Movies, parts.Shows, parts.Music} {
		if h.partOn(p) {
			return ""
		}
	}
	return "Movies, TV shows and Music are off"
}

func asksOff(h *Handlers, who webserver.Caller) string {
	if !h.partOn(parts.Wishlist) {
		return "Requests is off"
	}
	return ""
}

func needPerm(p webserver.Perm, base func(*Handlers, webserver.Caller) string) func(*Handlers, webserver.Caller) string {
	return func(h *Handlers, who webserver.Caller) string {
		if why := base(h, who); why != "" {
			return why
		}
		if !who.Has(p) {
			return "this person may not do that"
		}
		return ""
	}
}

func always(*Handlers, webserver.Caller) string { return "" }

// tools lists the tools in a fixed order.
func (h *Handlers) tools() []toolSpec {
	return []toolSpec{
		{name: "search_titles", title: "Search titles", about: "Find movies and shows in plain words, like \"funny 90s movies\" or \"space movie from 2014\". Returns Library titles and titles that could be asked for.",
			schema: object(map[string]any{"query": strProp("What to look for, in plain words.")}, "query"), off: mediaOff, run: toolSearch},
		{name: "library_lookup", title: "Look in the Library", about: "Check whether a movie, show or artist is in the Library, by name.",
			schema: object(map[string]any{"name": strProp("The name to look for."), "kind": map[string]any{"type": "string", "enum": []string{"all", "movie", "show", "music"}}}, "name"),
			off:    mediaOff, run: toolLibrary},
		{name: "calendar", title: "Calendar", about: "What comes out in the next days: movies and episodes of shows in the Library.",
			schema: object(map[string]any{"days": intProp("How many days ahead, 1 to 60 (default 14).")}), off: mediaOff, run: toolCalendar},
		{name: "downloads", title: "Downloads", about: "What is downloading now and how far along it is. Read only.",
			schema: object(map[string]any{}), off: always, run: toolDownloads},
		{name: "my_asks", title: "My requests", about: "The person's own requests and their state, and how many more they may ask for.",
			schema: object(map[string]any{}), off: asksOff, run: toolMyAsks},
		{name: "health", title: "Health", about: "Whether HolyArrs and the apps it uses are working.",
			schema: object(map[string]any{}), off: needPerm(webserver.PermManageLibrary, always), run: toolHealth},
		{name: "ask_for_title", title: "Ask for a title", action: true,
			about:  "Ask for a movie or show by its TMDB id. The first call returns a summary and a confirm_token; call again with the token after the person agrees.",
			schema: object(map[string]any{"tmdb_id": strProp("The TMDB id, like 693134."), "kind": map[string]any{"type": "string", "enum": []string{"movie", "show"}}, "confirm_token": tokProp}, "tmdb_id"),
			off:    needPerm(webserver.PermAsk, asksOff), run: toolAsk},
	}
}

// callTool runs one tool; wait is set when the key is over its action limit.
func (h *Handlers) callTool(r *http.Request, who webserver.Caller, name string, args map[string]any) (map[string]any, time.Duration) {
	i := slices.IndexFunc(h.tools(), func(t toolSpec) bool { return t.name == name })
	if i < 0 {
		return toolError("there is no tool called " + clean(name, 60)), 0
	}
	t := h.tools()[i]
	if why := t.off(h, who); why != "" {
		return toolError("this tool is not available: " + why + "."), 0
	}
	if t.action && str(args, "confirm_token") != "" {
		if ok, wait := h.Allow(&h.Acts, who.KeyID, ActsPerHour, time.Hour); !ok {
			return nil, wait
		}
	}
	out, err := t.run(h, r, who, args)
	if err != nil {
		var f failure
		if errors.As(err, &f) {
			return toolError(f.msg), 0
		}
		return toolError("something went wrong; see the HolyArrs log"), 0
	}
	data := Tidy(out, 0)
	m, ok := data.(map[string]any)
	if !ok {
		m = map[string]any{"items": data}
	}
	text, _ := json.Marshal(m)
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}, "structuredContent": m, "isError": false}, 0
}

func toolError(msg string) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": msg}}, "isError": true}
}

// call runs one app route as who with only scope, the way the app would.
func (h *Handlers) call(r *http.Request, who webserver.Caller, scope, method, path string, body any) (int, any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	who.Scopes = []string{scope}
	req := httptest.NewRequestWithContext(webserver.ContextWith(r.Context(), who), method, path, rd)
	req.RemoteAddr, req.Host = r.RemoteAddr, r.Host
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	if h.Inner == nil {
		return http.StatusServiceUnavailable, nil
	}
	h.Inner.ServeHTTP(rec, req)
	var v any
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return rec.Code, v
}

// read is call for a GET that must answer 200.
func (h *Handlers) read(r *http.Request, who webserver.Caller, path string) (any, error) {
	code, v := h.call(r, who, webserver.ScopeRead, http.MethodGet, path, nil)
	if code != http.StatusOK {
		return nil, failure{routeError(code, v)}
	}
	return v, nil
}

// routeError says why a route refused, in its own words when it gave any.
func routeError(code int, v any) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m["error"].(string); ok && s != "" {
			return clean(s, TextMax)
		}
	}
	return fmt.Sprintf("HolyArrs answered %d", code)
}

func str(args map[string]any, k string) string {
	switch v := args[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	}
	return ""
}

func num(args map[string]any, k string) int64 {
	n, _ := strconv.ParseInt(str(args, k), 10, 64)
	return n
}

func toolSearch(h *Handlers, r *http.Request, who webserver.Caller, args map[string]any) (any, error) {
	q := str(args, "query")
	if q == "" {
		return nil, failure{"send a query"}
	}
	return h.read(r, who, "/api/v1/search?q="+url.QueryEscape(clean(q, 200)))
}

func toolLibrary(h *Handlers, r *http.Request, who webserver.Caller, args map[string]any) (any, error) {
	n := str(args, "name")
	if n == "" {
		return nil, failure{"send a name"}
	}
	kind := str(args, "kind")
	if kind == "" {
		kind = "all"
	}
	return h.read(r, who, "/api/v1/library/search?q="+url.QueryEscape(clean(n, 200))+"&kind="+url.QueryEscape(kind))
}

func toolCalendar(h *Handlers, r *http.Request, who webserver.Caller, args map[string]any) (any, error) {
	days := num(args, "days")
	if days < 1 || days > 60 {
		days = 14
	}
	from := h.clock().UTC()
	to := from.AddDate(0, 0, int(days))
	return h.read(r, who, "/api/v1/calendar?from="+from.Format("2006-01-02")+"&to="+to.Format("2006-01-02"))
}

func toolDownloads(h *Handlers, r *http.Request, who webserver.Caller, _ map[string]any) (any, error) {
	return h.read(r, who, "/api/v1/downloads/view")
}

// toolMyAsks is the person's own requests only: other people's requests
// stay off AI connections.
func toolMyAsks(h *Handlers, r *http.Request, who webserver.Caller, _ map[string]any) (any, error) {
	v, err := h.read(r, who, "/api/v1/asks?mine=1")
	if err != nil {
		return nil, err
	}
	out := map[string]any{"asks": v}
	if m, ok := v.(map[string]any); ok {
		out["asks"] = m["asks"]
	}
	if who.Has(webserver.PermAsk) {
		if l, err := h.read(r, who, "/api/v1/asks/limit"); err == nil {
			out["limit"] = l
		}
	}
	return out, nil
}

func toolHealth(h *Handlers, r *http.Request, who webserver.Caller, _ map[string]any) (any, error) {
	return h.read(r, who, "/api/v1/health/states")
}

// confirmStep is the first or second call of an action. On the first it
// returns a token to hand back; on the second it checks the token and
// says to go ahead.
func (h *Handlers) confirmStep(who webserver.Caller, tool string, args map[string]any, summary map[string]any) (map[string]any, bool, error) {
	key := tool + "|" + str(args, "tmdb_id") + "|" + str(args, "kind") + "|" + str(args, "ask_id") + "|" + str(args, "note")
	now := h.clock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tokens == nil {
		h.tokens = map[string]token{}
	}
	for k, t := range h.tokens {
		if now.After(t.until) {
			delete(h.tokens, k)
		}
	}
	tok := str(args, "confirm_token")
	if tok == "" {
		if len(h.tokens) >= tokensMax {
			return nil, false, failure{"too many actions are waiting; try again in a few minutes"}
		}
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		tok = hex.EncodeToString(b)
		h.tokens[tok] = token{user: who.UserID, key: who.KeyID, tool: tool, args: key, until: now.Add(confirmFor)}
		summary["confirm_token"] = tok
		summary["expires_in_s"] = int(confirmFor.Seconds())
		summary["next"] = "Show this to the person. If they agree, call " + tool + " again with the same arguments and this confirm_token."
		return summary, false, nil
	}
	t, ok := h.tokens[tok]
	if !ok || t.user != who.UserID || t.key != who.KeyID {
		return nil, false, failure{"that confirm_token is unknown, used or expired; call again without it to get a new one"}
	}
	if t.tool != tool || t.args != key {
		return nil, false, failure{"that confirm_token was given for something else; call again without it"}
	}
	delete(h.tokens, tok)
	return nil, true, nil
}

func toolAsk(h *Handlers, r *http.Request, who webserver.Caller, args map[string]any) (any, error) {
	id := str(args, "tmdb_id")
	if id == "" || len(id) > 12 || strings.Trim(id, "0123456789") != "" {
		return nil, failure{"send tmdb_id as digits, like 693134"}
	}
	kind := str(args, "kind")
	if kind == "" {
		kind = "movie"
	}
	if kind != "movie" && kind != "show" {
		return nil, failure{"kind is movie or show"}
	}
	if p, _ := parts.ForTitleKind(kind); !h.partOn(p) {
		return nil, failure{p.Label() + " is off"}
	}
	summary := map[string]any{"tmdb_id": id, "kind": kind}
	if str(args, "confirm_token") == "" {
		// The details page first, then the lookup the add screens use.
		lookup := map[string]string{"movie": "/api/v1/titles/lookup?term=tmdb:", "show": "/api/v1/shows/lookup?term=tmdb:"}[kind]
		for _, path := range []string{"/api/v1/metadata/" + kind + "/" + id, lookup + id} {
			v, err := h.read(r, who, path)
			if m, ok := v.(map[string]any); ok && m["name"] == nil {
				for _, k := range []string{"movies", "shows", "results"} {
					if l, ok := m[k].([]any); ok && len(l) > 0 {
						v = l[0]
					}
				}
			}
			if m, ok := v.(map[string]any); ok && err == nil && m["name"] != nil {
				for _, k := range []string{"name", "year", "overview"} {
					if m[k] != nil {
						summary[k] = m[k]
					}
				}
				break
			}
		}
		if summary["name"] == nil {
			summary["note"] = "The name could not be looked up; check the TMDB id with the person."
		}
		summary["will_do"] = "Ask for this " + kind + " as " + who.Name + "."
	}
	out, goAhead, err := h.confirmStep(who, "ask_for_title", args, summary)
	if err != nil || !goAhead {
		return out, err
	}
	code, v := h.call(r, who, webserver.ScopeWrite, http.MethodPost, "/api/v1/asks", map[string]any{"id": "tmdb:" + id, "kind": kind})
	return h.acted(r, who, "ask_for_title", args, "ai_ask_made", "asked for", code, v)
}

// acted writes the action to History and answers it.
func (h *Handlers) acted(r *http.Request, who webserver.Caller, tool string, args map[string]any, kind, did string, code int, v any) (any, error) {
	ok := code >= 200 && code < 300
	name := ""
	var titleID *int64
	askID := int64(0)
	if m, isMap := v.(map[string]any); isMap {
		if ask, isAsk := m["ask"].(map[string]any); isAsk {
			name, _ = ask["name"].(string)
			if f, isNum := ask["id"].(float64); isNum {
				askID = int64(f)
			}
			if f, isNum := ask["title_id"].(float64); isNum && f > 0 {
				t := int64(f)
				titleID = &t
			}
		}
	}
	if name == "" {
		name = "a title"
	}
	logged := map[string]any{}
	for k, val := range args {
		if k != "confirm_token" {
			logged[k] = Tidy(val, 4)
		}
	}
	if b, _ := json.Marshal(logged); len(b) > argsLogLimit {
		logged = map[string]any{"cut": true}
	}
	client := h.clientOf(who.KeyID)
	summary := fmt.Sprintf("AI (%s, key %s) %s %s for %s", client, clean(who.KeyLabel, 64), did, clean(name, 120), who.Name)
	data := map[string]any{"key_id": who.KeyID, "key": clean(who.KeyLabel, 64), "client": client, "tool": tool, "args": logged, "ok": ok}
	if askID > 0 {
		data["ask_id"] = askID
	}
	if !ok {
		kind = "ai_action_failed"
		summary = fmt.Sprintf("AI (%s, key %s) could not use %s for %s", client, clean(who.KeyLabel, 64), tool, who.Name)
		data["error"] = routeError(code, v)
	}
	_ = h.Store.AddEvent(r.Context(), &domain.Event{Actor: domain.Actor{Kind: "ai", ID: who.UserID}, Kind: kind, TitleID: titleID, Summary: summary, Data: data})
	if !ok {
		return nil, failure{routeError(code, v)}
	}
	return map[string]any{"done": true, "result": v}, nil
}
