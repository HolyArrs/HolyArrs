package accounts

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// pluginTokenPrefix starts every plugin token, so the bearer header tells
// it apart from an AI client's key.
const pluginTokenPrefix = "hap_"

// pluginBearer is the plugin token a request carries as
// "Authorization: Bearer hap_...", or "".
func pluginBearer(r *http.Request) string {
	v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if v = strings.TrimSpace(v); !ok || !strings.HasPrefix(v, pluginTokenPrefix) {
		return ""
	}
	return v
}

// pluginRate is how many calls one plugin may make per minute.
const pluginRate = 600

type pluginToken struct {
	id     string
	token  string
	scopes []string
	window time.Time
	calls  int
	// ctx ends when the token does; calls in flight end with it.
	ctx    context.Context
	cancel context.CancelFunc
}

func newPluginToken(id, tok string, scopes []string) *pluginToken {
	ctx, cancel := context.WithCancel(context.Background())
	return &pluginToken{id: id, token: tok, scopes: slices.Clone(scopes), ctx: ctx, cancel: cancel}
}

var (
	pluginTokMu sync.Mutex
	pluginToks  = map[string]*pluginToken{} // by plugin id
	callToks    = map[string]*pluginToken{} // by token: one tool call each
)

// IssuePluginToken makes a fresh token for a plugin start with its granted
// scopes. It lives only in memory and replaces the plugin's older token.
func IssuePluginToken(id string, scopes []string) string {
	t := newPluginToken(id, pluginTokenPrefix+rand.Text(), scopes)
	pluginTokMu.Lock()
	if old := pluginToks[id]; old != nil {
		old.cancel()
	}
	pluginToks[id] = t
	pluginTokMu.Unlock()
	return t.token
}

// RevokePluginToken ends a plugin's token (stop, block, remove).
func RevokePluginToken(id string) {
	pluginTokMu.Lock()
	if t := pluginToks[id]; t != nil {
		t.cancel()
	}
	delete(pluginToks, id)
	for k, t := range callToks {
		if t.id == id {
			t.cancel()
			delete(callToks, k)
		}
	}
	pluginTokMu.Unlock()
}

// IssuePluginCallToken makes a token for one assistant tool call: it holds
// only scopes (the overlap of the plugin's grants and the person's rights)
// and ends with end, or with the plugin's own token.
func IssuePluginCallToken(id string, scopes []string) (tok string, end func()) {
	tok = pluginTokenPrefix + rand.Text()
	pluginTokMu.Lock()
	if pluginToks[id] != nil {
		callToks[tok] = newPluginToken(id, tok, scopes)
	}
	pluginTokMu.Unlock()
	return tok, func() {
		pluginTokMu.Lock()
		if t := callToks[tok]; t != nil {
			t.cancel()
		}
		delete(callToks, tok)
		pluginTokMu.Unlock()
	}
}

// pluginCaller is the caller a token stands for: the plugin itself, no
// person, no role, only its scopes.
func pluginCaller(tok string) (webserver.Caller, bool) {
	who, _, ok := pluginCallerCtx(tok)
	return who, ok
}

// pluginCallerCtx is pluginCaller plus the token's context, which ends the
// moment the token is revoked.
func pluginCallerCtx(tok string) (webserver.Caller, context.Context, bool) {
	if tok == "" {
		return webserver.Caller{}, nil, false
	}
	pluginTokMu.Lock()
	defer pluginTokMu.Unlock()
	found := callToks[tok]
	if found == nil {
		for _, t := range pluginToks {
			if subtle.ConstantTimeCompare([]byte(t.token), []byte(tok)) == 1 {
				found = t
			}
		}
	}
	if found == nil {
		return webserver.Caller{}, nil, false
	}
	return webserver.Caller{Via: "plugin", PluginID: found.id, Name: "plugin " + found.id, Scopes: slices.Clone(found.scopes)}, found.ctx, true
}

// PluginTokenContext ends when id's current token does; nil when it has none.
func PluginTokenContext(id string) context.Context {
	pluginTokMu.Lock()
	defer pluginTokMu.Unlock()
	if t := pluginToks[id]; t != nil {
		return t.ctx
	}
	return nil
}

// pluginCost weighs routes that fan out to many outside calls (indexers,
// metadata), so a plugin can't get the owner banned by an indexer.
var pluginCost = map[string]int{
	"POST /api/v1/titles/refresh":           60,
	"POST /api/v1/search/missing":           120,
	"POST /api/v1/schedules/{kind}/run":     120,
	"POST /api/v1/subtitles/missing/search": 60,
}

const (
	callOK = iota
	callTooMany
	callRevoked
)

// pluginAllowCall counts a call of the given cost (1 when 0) against the
// plugin's per-minute budget.
func pluginAllowCall(id string, cost int, now time.Time) int {
	if cost < 1 {
		cost = 1
	}
	pluginTokMu.Lock()
	defer pluginTokMu.Unlock()
	t := pluginToks[id]
	if t == nil {
		return callRevoked
	}
	if now.Sub(t.window) >= time.Minute {
		t.window, t.calls = now, 0
	}
	if t.calls+cost > pluginRate {
		return callTooMany
	}
	t.calls += cost
	return callOK
}

// gatePlugin lets a plugin through only on a route whose plugin scope it
// holds; role checks don't apply to it, its scopes do. Every call is logged.
func gatePlugin(w http.ResponseWriter, r *http.Request, who webserver.Caller, pattern string, next http.Handler) {
	outcome := "refused"
	defer func() { recordPluginCall(who.PluginID, r.Method, r.URL.Path, outcome, time.Now()) }()
	need := PluginScopes[pattern]
	if need == "" || PluginClosed[pattern] || !PluginScopesCover(who.Scopes, need) {
		log.Printf("plugin %s: %s %s refused", who.PluginID, r.Method, r.URL.Path)
		msg := "plugins can't use this route"
		if need != "" {
			msg = "this plugin was not given the " + need + " permission"
		}
		fail(w, http.StatusForbidden, msg)
		return
	}
	if pluginForPerson(pattern) {
		// These act for one person (their requests, limits and follows): only
		// for someone who linked this plugin, with their own rights too.
		if who.UserID == 0 {
			fail(w, http.StatusForbidden, "this acts for a person: send "+PluginForUserHeader+" for someone who linked this plugin")
			return
		}
		perm, known := RoutePerms[pattern]
		if !known {
			perm = webserver.NeedOwner
		}
		if !who.Has(perm) {
			fail(w, http.StatusForbidden, "that person may not do this")
			return
		}
	} else if who.UserID != 0 {
		fail(w, http.StatusForbidden, "a plugin acts for a person only on requests and follows")
		return
	}
	if pluginTouchesFiles(pattern, need) && PluginHandsOff != nil && PluginHandsOff() {
		fail(w, http.StatusForbidden, "hands-off mode is on, so plugins can't change media files")
		return
	}
	switch pluginAllowCall(who.PluginID, pluginCost[pattern], time.Now()) {
	case callRevoked:
		fail(w, http.StatusUnauthorized, "plugin token not accepted")
		return
	case callTooMany:
		outcome = "limited"
		log.Printf("plugin %s: %s %s over its limit", who.PluginID, r.Method, r.URL.Path)
		fail(w, http.StatusTooManyRequests, "this plugin is calling too often; try again in a minute")
		return
	}
	if !enterPlugin(who.PluginID) {
		outcome = "limited"
		fail(w, http.StatusTooManyRequests, "this plugin has too many calls open; try again when one ends")
		return
	}
	defer leavePlugin(who.PluginID)
	outcome = "allowed"
	inner := r.WithContext(database.WithViaPlugin(r.Context(), who.PluginID))
	defer func() { r.Pattern = inner.Pattern }() // route-hit records read the outer request
	if who.UserID != 0 {
		log.Printf("plugin %s for user %d: %s %s", who.PluginID, who.UserID, r.Method, r.URL.Path)
	} else {
		log.Printf("plugin %s: %s %s", who.PluginID, r.Method, r.URL.Path)
	}
	next.ServeHTTP(w, inner)
}

// pluginsLive says whether any plugin holds a token or a forwarding mark.
func pluginsLive() bool {
	pluginTokMu.Lock()
	n := len(pluginToks)
	pluginTokMu.Unlock()
	markMu.Lock()
	n += len(marks)
	markMu.Unlock()
	return n > 0
}

// pluginTouchesFiles says whether a route changes media files: any files,
// library or downloads delete, any change under files, and the library
// changes that move, rename, adopt, clean or keep files. Hands-off mode
// refuses these to plugins.
func pluginTouchesFiles(pattern, scope string) bool {
	if strings.HasSuffix(scope, ".read") {
		return false
	}
	if strings.HasPrefix(scope, "files.") || strings.HasSuffix(scope, ".delete") {
		return true
	}
	if scope != "library.change" {
		return false
	}
	_, path, _ := strings.Cut(pattern, " ")
	for _, w := range []string{"/rename", "/move", "/adopt", "/subtitles/clean", "/keep-and-add", "/leftovers", "/rescan"} {
		if strings.Contains(path, w) {
			return true
		}
	}
	return false
}

// PluginHandsOff reports whether hands-off mode is on; set by the app. While
// it is on, no plugin may change media files.
var PluginHandsOff func() bool

// PluginTokenScopes are the scopes of id's live token, none when it has
// none (stopped, blocked or removed).
func PluginTokenScopes(id string) []string {
	pluginTokMu.Lock()
	defer pluginTokMu.Unlock()
	if t := pluginToks[id]; t != nil {
		return slices.Clone(t.scopes)
	}
	return nil
}

// PersonName is a person as a plugin with requests.users.read sees them.
type PersonName struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PeopleNames lists the active people, id and name only.
func (a *Auth) PeopleNames(ctx context.Context) ([]PersonName, error) {
	return personNames(ctx, a.s.Read())
}

// pluginForPerson says whether a route acts for one person: making,
// removing or deciding requests, request limits, and follows.
func pluginForPerson(pattern string) bool {
	method, path, _ := strings.Cut(pattern, " ")
	if method == http.MethodGet {
		return path == "/api/v1/my/follows" || strings.HasSuffix(path, "/follow")
	}
	return strings.HasPrefix(path, "/api/v1/asks") || strings.HasPrefix(path, "/api/v1/ask-limits") ||
		strings.HasPrefix(path, "/api/v1/turned-down") || strings.HasSuffix(path, "/follow")
}
