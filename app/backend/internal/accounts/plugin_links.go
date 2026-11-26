package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// PluginForUserHeader names the person a plugin acts for on a request,
// follow or limit route. It counts only for a person who linked that plugin
// to their own account.
const PluginForUserHeader = "X-Holyarrs-For-User"

// linkLife is how long a link code works.
const linkLife = 10 * time.Minute

type linkCode struct {
	plugin, external string
	until            time.Time
}

var (
	linkMu    sync.Mutex
	linkCodes = map[string]linkCode{}
	linkFails = map[int64][]time.Time{} // wrong codes per person
)

// linkFailMax is how many wrong codes one person may type in ten minutes.
const linkFailMax = 10

// linkFailed records a wrong code; tooManyLinkFails says whether the person
// is over the limit now.
func linkFailed(user int64, now time.Time) {
	linkMu.Lock()
	linkFails[user] = append(recentFails(user, now), now)
	linkMu.Unlock()
}

func tooManyLinkFails(user int64, now time.Time) bool {
	linkMu.Lock()
	defer linkMu.Unlock()
	return len(recentFails(user, now)) >= linkFailMax
}

// recentFails is user's wrong codes in the last ten minutes; linkMu held.
func recentFails(user int64, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range linkFails[user] {
		if now.Sub(t) < linkLife {
			keep = append(keep, t)
		}
	}
	linkFails[user] = keep
	return keep
}

// newLinkCode makes a code like 4821-0937 a plugin shows the person (in a
// chat, say); they type it in My account to link that plugin to themselves.
func newLinkCode(plugin, external string, now time.Time) string {
	linkMu.Lock()
	defer linkMu.Unlock()
	for k, v := range linkCodes {
		if now.After(v.until) || v.plugin == plugin && v.external == external {
			delete(linkCodes, k)
		}
	}
	for {
		n, _ := rand.Int(rand.Reader, big.NewInt(100000000))
		s := strconv.FormatInt(n.Int64()+100000000, 10)[1:]
		code := s[:4] + "-" + s[4:]
		if _, taken := linkCodes[code]; !taken {
			linkCodes[code] = linkCode{plugin: plugin, external: external, until: now.Add(linkLife)}
			return code
		}
	}
}

// takeLinkCode uses a code once.
func takeLinkCode(code string, now time.Time) (linkCode, bool) {
	code = strings.TrimSpace(code)
	if len(code) == 8 && !strings.Contains(code, "-") {
		code = code[:4] + "-" + code[4:]
	}
	linkMu.Lock()
	c, ok := linkCodes[code]
	delete(linkCodes, code)
	linkMu.Unlock()
	if !ok || now.After(c.until) || PluginTokenScopes(c.plugin) == nil {
		return linkCode{}, false
	}
	return c, true
}

func (a *Auth) pluginLinkRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/plugin-hooks/link", a.pluginLinkCode)
	m.HandleFunc("GET /api/v1/my/plugin-links", a.myPluginLinks)
	m.HandleFunc("POST /api/v1/my/plugin-links", a.addMyPluginLink)
	m.HandleFunc("DELETE /api/v1/my/plugin-links/{plugin}", a.removeMyPluginLink)
}

// pluginLinkCode: a plugin asks for a code to show one outside account (a
// chat user); only a running plugin may.
func (a *Auth) pluginLinkCode(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "plugin" || who.UserID != 0 {
		fail(w, http.StatusForbidden, "only a running plugin calls this")
		return
	}
	var in struct {
		External string `json:"external"`
	}
	if !webserver.Decode(w, r, &in, `send {"external": "<who this is in your service>"}`) {
		return
	}
	ext := strings.TrimSpace(in.External)
	if ext == "" || utf8.RuneCountInString(ext) > 100 || strings.ContainsAny(ext, "<>\x00") {
		fail(w, http.StatusBadRequest, `send {"external": "<who this is in your service>"}`)
		return
	}
	code := newLinkCode(who.PluginID, ext, a.now())
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "expires": a.now().Add(linkLife).UTC()})
}

func (a *Auth) myPluginLinks(w http.ResponseWriter, r *http.Request) {
	me, ok := webserver.From(r.Context())
	if !ok || me.Via == "plugin" {
		fail(w, http.StatusForbidden, "only you can see your plugin links")
		return
	}
	ls, err := pluginLinksOf(r.Context(), a.s.Read(), me.UserID)
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": ls})
}

func (a *Auth) addMyPluginLink(w http.ResponseWriter, r *http.Request) {
	me, ok := webserver.From(r.Context())
	if !ok || me.Via == "plugin" || me.Via == "local_network" || me.UserID == 0 {
		fail(w, http.StatusForbidden, "sign in to link a plugin to your account")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !webserver.Decode(w, r, &in, `send {"code": "1234-5678"}`) {
		return
	}
	if tooManyLinkFails(me.UserID, a.now()) {
		fail(w, http.StatusTooManyRequests, "too many wrong codes; try again in ten minutes")
		return
	}
	c, ok := takeLinkCode(in.Code, a.now())
	if !ok {
		linkFailed(me.UserID, a.now())
		fail(w, http.StatusBadRequest, "that code is wrong or has run out; ask the plugin for a new one")
		return
	}
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return addPluginLink(r.Context(), tx, c.plugin, me.UserID, c.external, database.FormatTime(a.now()))
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	log.Printf("accounts: user %d linked plugin %s", me.UserID, c.plugin)
	a.myPluginLinks(w, r)
}

func (a *Auth) removeMyPluginLink(w http.ResponseWriter, r *http.Request) {
	me, ok := webserver.From(r.Context())
	if !ok || me.Via == "plugin" {
		fail(w, http.StatusForbidden, "only you can remove your plugin links")
		return
	}
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return removePluginLink(r.Context(), tx, r.PathValue("plugin"), me.UserID)
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.myPluginLinks(w, r)
}

// pluginForUser turns a plugin caller into the person it acts for, when the
// request names one in PluginForUserHeader and that person linked the plugin:
// their own role, permissions and limits, with the plugin's scopes, recorded
// as via the plugin.
func (a *Auth) pluginForUser(ctx context.Context, r *http.Request, who webserver.Caller) (webserver.Caller, error) {
	v := r.Header.Get(PluginForUserHeader)
	if v == "" {
		return who, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return who, errors.New("bad " + PluginForUserHeader)
	}
	linked, err := pluginLinked(ctx, a.s.Read(), who.PluginID, id)
	if err != nil || !linked {
		return who, errors.New("that person has not linked this plugin to their account")
	}
	var p webserver.Caller
	perms, err := activeCaller(ctx, a.s.Read(), id, &p)
	if err != nil {
		return who, errors.New("that person has not linked this plugin to their account")
	}
	a.fill(&p, perms)
	p.Via, p.PluginID, p.Scopes = "plugin", who.PluginID, who.Scopes
	p.Name = p.Name + " via " + who.PluginID
	return p, nil
}
