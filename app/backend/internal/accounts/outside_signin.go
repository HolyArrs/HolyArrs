package accounts

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/features"
)

// Sign-in through an outside account: Plex (a PIN that the person confirms
// on Plex's own site) and Jellyfin or Emby (name and password checked by the
// owner's server). A first sign-in makes a member when the owner lets them
// join; later sign-ins find the linked user.

// Sign-in setting keys.
const (
	keyRequireAdmin2FA = "require_two_factor_admins"
	keyClientID        = "client_id"
)

// Providers.
const (
	ProviderPlex     = "plex"
	ProviderJellyfin = "jellyfin"
	ProviderEmby     = "emby"
)

// Join rules for people who are not linked yet.
const (
	JoinNobody   = "nobody"   // only people already linked
	JoinList     = "list"     // only the names or emails on the owner's list
	JoinEveryone = "everyone" // everyone the server lets in
)

const (
	pinLife    = 10 * time.Minute
	maxPins    = 1000
	pinsPerIP  = 5
	outsideMax = 1 << 20 // bytes read from an outside answer
	product    = "HolyArrs"
)

// WayView is one outside sign-in way as settings show it.
type WayView struct {
	On       bool   `json:"on"`
	URL      string `json:"url,omitempty"`       // Jellyfin and Emby
	ServerID string `json:"server_id,omitempty"` // Plex: the server people must share
	Join     string `json:"join"`
	Allow    string `json:"allow"` // comma-separated names or emails for JoinList
}

func (a *Auth) way(ctx context.Context, p string) WayView {
	v := WayView{
		On:       a.signInSetting(ctx, p+"_on") == "true" && (p != ProviderEmby || startup.FeatureOn(features.EmbySignIn)),
		URL:      a.signInSetting(ctx, p+"_url"),
		ServerID: a.signInSetting(ctx, p+"_server_id"),
		Join:     a.signInSetting(ctx, p+"_join"),
		Allow:    a.signInSetting(ctx, p+"_allow"),
	}
	if v.Join == "" {
		v.Join = JoinNobody
	}
	return v
}

// clientID is this HolyArrs's id toward Plex and media servers, made once.
func (a *Auth) clientID(ctx context.Context) string {
	if id := a.signInSetting(ctx, keyClientID); id != "" {
		return id
	}
	id := "holyarrs-" + token(12)
	_ = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		return database.PutSettingIfMissing(tx, settingsGroup, signInPrefix+keyClientID, id, database.System, a.now())
	})
	return a.signInSetting(ctx, keyClientID)
}

func (a *Auth) outsideRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/plex/start", a.plexStart)
	m.HandleFunc("POST /api/v1/auth/plex/finish", a.plexFinish)
	m.HandleFunc("POST /api/v1/auth/media-server/{kind}", a.mediaServerLogin)
	a.setupOutsideRoutes(m)
	a.inviteOutsideRoutes(m)
}

// ways lists the sign-in ways the sign-in page offers.
func (a *Auth) ways(ctx context.Context) []string {
	out := []string{"password"}
	for _, p := range []string{ProviderPlex, ProviderJellyfin, ProviderEmby} {
		if a.way(ctx, p).On {
			out = append(out, p)
		}
	}
	sso := a.ssoNow()
	if sso.oidc != nil {
		out = append(out, "oidc")
	}
	if sso.forward != nil {
		out = append(out, "forward_auth")
	}
	return out
}

func (a *Auth) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nets := []string{}
	if l := a.localNets.Load(); l != nil {
		nets = l.list
	}
	seen, untrusted := a.seenAddress(r)
	writeJSON(w, http.StatusOK, map[string]any{
		settingSkipLocal:   a.skipLocal.Load(),
		keyLocalNets:       nets,
		keyRequireAdmin2FA: a.requireAdmin2FA.Load(),
		keyMinLength:       MinPasswordLength(ctx, a.s.Read()),
		ProviderPlex:       a.way(ctx, ProviderPlex),
		ProviderJellyfin:   a.way(ctx, ProviderJellyfin),
		ProviderEmby:       a.way(ctx, ProviderEmby),
		"plex_owner":       a.plexOwner(ctx),
		keyAllowedHosts:    a.savedHostList(),
		// For the skip card: the address this browser shows up as, listed
		// networks that look like a container gateway, and whether a proxy
		// in front is not trusted (then the skip never applies).
		"seen_address":     seen,
		"gateway_networks": gatewayNets(nets, seen),
		"proxy_untrusted":  untrusted,
	})
}

func (a *Auth) putSettings(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		SkipLocal    *bool    `json:"skip_login_local_network"`
		LocalNets    []string `json:"local_networks"`
		RequireAdmin *bool    `json:"require_two_factor_admins"`
		MinPassword  *int     `json:"min_password_length"`
		Plex         *WayView `json:"plex"`
		Jellyfin     *WayView `json:"jellyfin"`
		Emby         *WayView `json:"emby"`
		// AllowedHosts are the host names saved in Settings; names from
		// HOLYARRS_ALLOWED_HOSTS are not listed and stay.
		AllowedHosts *[]string `json:"allowed_hosts"`
		// ConfirmEveryone must be true to open a way to everyone the
		// server lets in, when it was not open to everyone already.
		ConfirmEveryone bool `json:"confirm_everyone"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil ||
		(body.SkipLocal == nil && body.LocalNets == nil && body.RequireAdmin == nil && body.MinPassword == nil && body.Plex == nil && body.Jellyfin == nil && body.Emby == nil && body.AllowedHosts == nil) {
		fail(w, http.StatusBadRequest, `send {"skip_login_local_network": true|false, "local_networks": ["192.168.1.0/24"], "require_two_factor_admins": true|false, "min_password_length": 4..64, "plex": {...}, "jellyfin": {...}, "emby": {...}}`)
		return
	}
	if body.SkipLocal != nil && *body.SkipLocal {
		kids, err := countKids(r.Context(), a.s.Read())
		if err != nil {
			serverError(w, "sign-in settings", err)
			return
		}
		if kids > 0 {
			fail(w, http.StatusBadRequest, "Skipping sign-in on the local network cannot be on while a Kid account has an age limit: anyone at home would sign in as the owner. Remove the limits first.")
			return
		}
		// An open invite with an age limit makes a Kid once it is used.
		expiries, err := kidInviteExpiries(r.Context(), a.s.Read())
		if err != nil {
			serverError(w, "sign-in settings", err)
			return
		}
		open := false
		for _, expires := range expiries {
			if a.now().Before(parseTime(expires)) {
				open = true
			}
		}
		if open {
			fail(w, http.StatusBadRequest, "Skipping sign-in on the local network cannot be on while an open invite has an age limit: the Kid it makes would sign in as the owner at home. Withdraw the invite or let it run out first.")
			return
		}
	}
	// Skipping needs at least one listed network; an empty list skips nowhere.
	nets := a.localNets.Load()
	if body.LocalNets != nil {
		var err error
		if nets, err = parseLocalNets(body.LocalNets); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	skip := a.skipLocal.Load()
	if body.SkipLocal != nil {
		skip = *body.SkipLocal
	}
	if (body.SkipLocal != nil || body.LocalNets != nil) && skip && (nets == nil || len(nets.list) == 0) {
		fail(w, http.StatusBadRequest, "List at least one local network first, like 192.168.1.0/24: sign-in is skipped only there.")
		return
	}
	set := map[string]string{}
	if body.SkipLocal != nil {
		set["this_app/"+settingSkipLocal] = strconv.FormatBool(*body.SkipLocal)
	}
	if body.LocalNets != nil {
		v, _ := json.Marshal(nets.list)
		set[settingsGroup+"/"+signInPrefix+keyLocalNets] = string(v)
	}
	if body.RequireAdmin != nil {
		set[settingsGroup+"/"+signInPrefix+keyRequireAdmin2FA] = strconv.FormatBool(*body.RequireAdmin)
	}
	if body.MinPassword != nil {
		if *body.MinPassword < MinPassword || *body.MinPassword > MaxMinPassword {
			fail(w, http.StatusBadRequest, fmt.Sprintf("the shortest password allowed must be %d to %d characters", MinPassword, MaxMinPassword))
			return
		}
		set[settingsGroup+"/"+signInPrefix+keyMinLength] = strconv.Itoa(*body.MinPassword)
	}
	for p, v := range map[string]*WayView{ProviderPlex: body.Plex, ProviderJellyfin: body.Jellyfin, ProviderEmby: body.Emby} {
		if v == nil {
			continue
		}
		if v.Join == "" {
			v.Join = JoinNobody
		}
		if v.Join != JoinNobody && v.Join != JoinList && v.Join != JoinEveryone {
			fail(w, http.StatusBadRequest, "join must be nobody, list or everyone")
			return
		}
		v.URL = strings.TrimRight(strings.TrimSpace(v.URL), "/")
		if p != ProviderPlex && v.URL != "" {
			u, err := url.Parse(v.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
				fail(w, http.StatusBadRequest, p+" address must look like http://192.168.1.10:8096")
				return
			}
		}
		if v.Join == JoinEveryone && !body.ConfirmEveryone && a.way(r.Context(), p).Join != JoinEveryone {
			fail(w, http.StatusConflict, "letting everyone from "+p+" join needs confirming: send confirm_everyone")
			return
		}
		if p == ProviderPlex && v.Join == JoinEveryone && strings.TrimSpace(v.ServerID) == "" {
			// Without a server to share, everyone would mean every Plex account in the world.
			fail(w, http.StatusBadRequest, "to let everyone from Plex join, give the server id people must share")
			return
		}
		if p != ProviderPlex && v.On && v.URL == "" {
			fail(w, http.StatusBadRequest, "give the "+p+" server address first")
			return
		}
		g := settingsGroup + "/" + signInPrefix + p
		set[g+"_on"] = strconv.FormatBool(v.On)
		set[g+"_join"] = v.Join
		set[g+"_allow"] = truncate(strings.TrimSpace(v.Allow), 4000)
		if p == ProviderPlex {
			set[g+"_server_id"] = truncate(strings.TrimSpace(v.ServerID), 200)
		} else {
			set[g+"_url"] = truncate(v.URL, 500)
		}
	}
	var hosts []string
	if body.AllowedHosts != nil {
		// A browser session with a recent password only (Gate asks for the
		// password again); never an API key.
		if who, ok := webserver.From(r.Context()); ok && who.Via != "session" {
			fail(w, http.StatusForbidden, "change Allowed host names in the browser, signed in with your password")
			return
		}
		var err error
		if hosts, err = cleanHostList(*body.AllowedHosts); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		// Removing the name this browser uses would lock it out.
		if h := normHost(r.Host); h != "" && net.ParseIP(h) == nil && !a.hosts[h] && !slices.Contains(hosts, h) {
			fail(w, http.StatusConflict, "you opened HolyArrs as "+echoHost(r.Host)+"; keep that name in the list, or change the list from the IP address")
			return
		}
		v, _ := json.Marshal(hosts)
		set[settingsGroup+"/"+signInPrefix+keyAllowedHosts] = string(v)
	}
	by := webserver.Actor(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		for k, v := range set {
			grp, key, _ := strings.Cut(k, "/")
			if err := database.PutSetting(tx, grp, key, v, by, a.now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		serverError(w, "sign-in settings", err)
		return
	}
	if body.LocalNets != nil {
		a.localNets.Store(nets)
		log.Printf("auth: local networks set to %s", strings.Join(nets.list, ", "))
	}
	if body.SkipLocal != nil {
		a.skipLocal.Store(*body.SkipLocal)
		log.Printf("auth: skip sign-in on the local network set to %t", *body.SkipLocal)
	}
	if body.RequireAdmin != nil {
		a.requireAdmin2FA.Store(*body.RequireAdmin)
	}
	if body.AllowedHosts != nil {
		a.savedHosts.Store(&hosts)
		log.Printf("auth: allowed host names set to %s", strings.Join(hosts, ", "))
	}
	log.Printf("auth: sign-in settings changed by user %d", by.ID)
	a.getSettings(w, r)
}

// allowed says whether a not-yet-linked outside account may join.
func allowed(v WayView, names ...string) bool {
	switch v.Join {
	case JoinEveryone:
		return true
	case JoinList:
		for _, e := range strings.FieldsFunc(v.Allow, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' }) {
			for _, n := range names {
				if n != "" && strings.EqualFold(e, n) {
					return true
				}
			}
		}
	}
	return false
}

var errNotAllowed = errors.New("not allowed to join")
var errDisabled = errors.New("account disabled")
var errKidSkipLocal = errors.New("kid join while skipping local sign-in")

// linkedUser finds or makes the user for an outside account. A new person
// the server limits (kid) joins as a Kid, as an import would make them.
func (a *Auth) linkedUser(ctx context.Context, provider, extID, extName string, mayJoin, kid bool) (int64, string, error) {
	id, name, state, err := linkedUserRow(ctx, a.s.Read(), provider, extID)
	if err == nil {
		if state != "active" {
			// The member's name goes with the refusal, for the audit row only.
			return 0, name, errDisabled
		}
		return id, name, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}
	if !mayJoin {
		// The outside account's name (from the server) goes with the refusal.
		return 0, strings.TrimSpace(extName), errNotAllowed
	}
	u := domain.User{Role: webserver.RoleMember, State: "active", Perms: permsJSON(webserver.DefaultPerms(webserver.RoleMember))}
	if kid {
		if a.skipLocal.Load() {
			// A Kid could not be told apart from the owner at home.
			return 0, "", errKidSkipLocal
		}
		u.Perms, u.MaxRating = permsJSON([]webserver.Perm{webserver.PermAsk}), "PG"
	}
	base := outsideBase(provider, extName)
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		if name, err = freeName(tx, base); err != nil {
			return err
		}
		u.Name = name
		id, err = insertUserTx(tx, &u, domain.System)
		if err != nil {
			return err
		}
		return insertUserLink(ctx, tx, id, provider, extID, truncate(extName, 200), database.FormatTime(a.now()))
	})
	if err == nil {
		log.Printf("auth: %s account joined as member %q (%d), kid %t", provider, name, id, kid)
	}
	return id, name, err
}

// outsideBase is the name a new user from an outside account starts from.
func outsideBase(provider, extName string) string {
	base := strings.TrimSpace(extName)
	if base == "" {
		base = provider + "-user"
	}
	return truncate(base, 56)
}

// freeName is base, or base-2, base-3... when that name is taken.
func freeName(tx *sql.Tx, base string) (string, error) {
	name := base
	for i := 2; ; i++ {
		n, err := countUsersNamed(context.Background(), tx, name)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return name, nil
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// linkedName is the member an outside account belongs to, found through its
// link (by id, else by its name on that server), else the member with that
// name, else unknownName. The typed name is only matched, never stored.
func (a *Auth) linkedName(ctx context.Context, provider, extID, extName string) string {
	if extID != "" {
		if n, err := linkedUserName(ctx, a.s.Read(), provider, extID); err == nil {
			return n
		}
	}
	if n, err := linkedUserNameByExternalName(ctx, a.s.Read(), provider, extName); err == nil {
		return n
	}
	return a.knownName(ctx, extName)
}

// outsideName is an outside account's name for a failed sign-in row (it
// comes from the Plex, Jellyfin or Emby server, never typed by hand).
func outsideName(name string) string {
	if name == "" {
		return unknownName
	}
	return name
}

// outsideDone answers after an outside account checked out.
func (a *Auth) outsideDone(w http.ResponseWriter, r *http.Request, provider string, id int64, name string, err error, pl place) {
	switch {
	case errors.Is(err, errNotAllowed):
		log.Printf("auth: %s account not allowed to join, from %s", provider, r.RemoteAddr)
		a.failedEvent(r, outsideName(name), provider+" not allowed")
		fail(w, http.StatusForbidden, "this account is not allowed in yet; ask the owner")
	case errors.Is(err, errDisabled):
		a.failedEvent(r, outsideName(name), "turned off")
		fail(w, http.StatusForbidden, "this account is turned off; ask the owner")
	case errors.Is(err, errKidSkipLocal):
		log.Printf("auth: %s account has limits on the server and sign-in is skipped at home; not joined", provider)
		fail(w, http.StatusForbidden, "this account has an age limit, and it can't join while sign-in is skipped at home; ask the owner")
	case err != nil:
		serverError(w, provider+" sign-in", err)
	default:
		a.passedPlace(w, r, id, name, provider, pl)
	}
}

// fetch makes one call to an outside service and decodes a JSON answer.
func (a *Auth) fetch(ctx context.Context, method, u string, hdr map[string]string, body any, out any) (int, error) {
	return a.call(ctx, a.http, method, u, hdr, body, out)
}

// serverFetch is fetch for a Jellyfin or Emby server the admin typed in.
// It never follows a redirect, so a password is only sent to that address.
func (a *Auth) serverFetch(ctx context.Context, method, u string, hdr map[string]string, body any, out any) (int, error) {
	return a.call(ctx, a.server, method, u, hdr, body, out)
}

func (a *Auth) call(ctx context.Context, c *http.Client, method, u string, hdr map[string]string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, outsideMax))
	if err != nil {
		return resp.StatusCode, err
	}
	if out != nil && resp.StatusCode/100 == 2 && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("unreadable answer: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// pin is a Plex sign-in in progress.
type pin struct {
	ip string
	// alt is the other header's key when the proxy headers disagreed, so
	// rotating a forged header still counts toward the same caller.
	alt string
	// secret is also in a cookie on the browser that started the pin, so
	// another visitor behind the same address cannot finish it.
	secret  string
	expires time.Time
	// checked is the last time Plex was asked about this pin; checks
	// closer than pinCheckGap answer 429 (the page polls every 2 s).
	checked time.Time
	// purpose is "" for a sign-in, pinOwner for the owner linking in Settings,
	// pinMine for anyone linking their own Plex account from My account.
	purpose string
	userID  int64 // who started a pinMine pin; only they may finish it
}

const (
	pinOwner = "owner"
	pinMine  = "mine"
)

// pinCheckGap is the shortest time between two checks of one pin.
const pinCheckGap = time.Second

func (a *Auth) plexHeaders(ctx context.Context) map[string]string {
	return map[string]string{"X-Plex-Product": product, "X-Plex-Client-Identifier": a.clientID(ctx)}
}

func (a *Auth) plexStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !a.way(ctx, ProviderPlex).On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
	a.startPin(w, r, "")
}

// startPin asks Plex for a PIN and answers with the page to confirm it on.
func (a *Auth) startPin(w http.ResponseWriter, r *http.Request, purpose string) {
	ctx := r.Context()
	ip, alt, _ := a.clientKeys(r)
	now := a.now()
	a.mu.Lock()
	mine := 0
	for id, p := range a.pins {
		if now.After(p.expires) {
			delete(a.pins, id)
		} else if p.ip == ip || (alt != "" && p.alt == alt) {
			mine++
		}
	}
	busy := mine >= pinsPerIP || len(a.pins) >= maxPins
	a.mu.Unlock()
	if busy {
		fail(w, http.StatusTooManyRequests, "too many Plex sign-ins started; wait a few minutes")
		return
	}
	var got struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
	}
	code, err := a.fetch(ctx, http.MethodPost, a.plexURL+"/api/v2/pins?strong=true", a.plexHeaders(ctx), nil, &got)
	if err != nil || code/100 != 2 || got.ID == 0 || got.Code == "" {
		log.Printf("auth: plex pin: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not answer; try again in a moment")
		return
	}
	secret := token(24)
	a.mu.Lock()
	p := &pin{ip: ip, alt: alt, secret: secret, expires: now.Add(pinLife), purpose: purpose}
	if purpose == pinMine {
		p.userID = webserver.Actor(ctx).ID
	}
	a.pins[got.ID] = p
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: pinCookie(got.ID), Value: secret, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.behindHTTPS(r), MaxAge: int(pinLife / time.Second)})
	q := url.Values{"clientID": {a.clientID(ctx)}, "code": {got.Code}, "context[device][product]": {product}}
	writeJSON(w, http.StatusOK, map[string]any{"id": got.ID, "auth_url": a.plexApp + "/auth#?" + q.Encode()})
}

func (a *Auth) plexFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := a.way(ctx, ProviderPlex)
	if !v.On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
	tok, ok := a.checkPin(w, r, "")
	if !ok {
		return
	}
	user, hdr, ok := a.plexUser(ctx, w, tok)
	if !ok {
		return
	}
	extID := strconv.FormatInt(user.ID, 10)
	mayJoin := allowed(v, user.Username, user.Email)
	if mayJoin && v.ServerID != "" {
		// Only people who share the owner's server may join.
		var res []struct {
			ClientIdentifier string `json:"clientIdentifier"`
			Provides         string `json:"provides"`
			Owned            bool   `json:"owned"`
		}
		code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/resources?includeHttps=1", hdr, nil, &res)
		if err != nil || code/100 != 2 {
			log.Printf("auth: plex resources: status %d: %v", code, err)
			fail(w, http.StatusBadGateway, "Plex did not list your servers; try again")
			return
		}
		mayJoin = slices.ContainsFunc(res, func(x struct {
			ClientIdentifier string `json:"clientIdentifier"`
			Provides         string `json:"provides"`
			Owned            bool   `json:"owned"`
		}) bool {
			// A server the joiner owns is their own, not a share: anyone
			// can copy the owner's server id onto a server they run.
			return x.ClientIdentifier == v.ServerID && !x.Owned
		})
	}
	id, name, err := a.linkedUser(ctx, ProviderPlex, extID, user.Username, mayJoin, user.Restricted)
	if err == nil {
		a.keepPlexToken(ctx, id, extID, tok) // for their Plex watchlist
	}
	a.outsideDone(w, r, ProviderPlex, id, name, err, place{})
}

// plexMe is the Plex account a confirmed PIN belongs to.
type plexMe struct {
	ID       int64  `json:"id"`
	UUID     string `json:"uuid"`
	Username string `json:"username"`
	Email    string `json:"email"`
	// A managed Plex Home user with limits on what they may watch.
	Restricted bool `json:"restricted"`
}

// plexUser asks Plex who a confirmed PIN's token belongs to. It returns the
// headers with the token for further calls, or has answered 502.
func (a *Auth) plexUser(ctx context.Context, w http.ResponseWriter, tok string) (plexMe, map[string]string, bool) {
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var user plexMe
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/user", hdr, nil, &user)
	if err != nil || code/100 != 2 || user.ID == 0 {
		log.Printf("auth: plex user: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not say who you are; try again")
		return user, hdr, false
	}
	return user, hdr, true
}

// checkPin asks Plex whether the person confirmed the PIN. It returns the
// token once they did; otherwise it has answered (waiting, ended, too soon).
// A PIN started to link an account cannot sign anyone in, and the other way
// round; one started from My account is only finished by the same person.
func (a *Auth) checkPin(w http.ResponseWriter, r *http.Request, purpose string) (string, bool) {
	ctx := r.Context()
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"id": ...}`)
		return "", false
	}
	a.mu.Lock()
	p := a.pins[body.ID]
	ok := p != nil && p.purpose == purpose && (purpose != pinMine || p.userID == webserver.Actor(ctx).ID) && p.ip == a.clientKey(r) && a.now().Before(p.expires) && pinSecretMatches(r, body.ID, p.secret)
	soon := ok && !p.checked.IsZero() && a.now().Sub(p.checked) < pinCheckGap
	if ok && !soon {
		p.checked = a.now()
	}
	a.mu.Unlock()
	if !ok {
		fail(w, http.StatusNotFound, "this Plex sign-in has ended; start again")
		return "", false
	}
	if soon {
		w.Header().Set("Retry-After", "1")
		fail(w, http.StatusTooManyRequests, "checking too often; wait a second")
		return "", false
	}
	var pinState struct {
		AuthToken string `json:"authToken"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/pins/"+strconv.FormatInt(body.ID, 10), a.plexHeaders(ctx), nil, &pinState)
	if err != nil || code/100 != 2 {
		log.Printf("auth: plex pin check: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not answer; try again in a moment")
		return "", false
	}
	if pinState.AuthToken == "" {
		writeJSON(w, http.StatusAccepted, map[string]bool{"waiting": true})
		return "", false
	}
	a.mu.Lock()
	delete(a.pins, body.ID) // one pin, one use
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: pinCookie(body.ID), Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.behindHTTPS(r), MaxAge: -1})
	return pinState.AuthToken, true
}

// pinCookie names the cookie that holds one pin's secret.
func pinCookie(id int64) string { return "holyarrs_plex_" + strconv.FormatInt(id, 10) }

// pinSecretMatches reports whether the request carries the pin's cookie.
func pinSecretMatches(r *http.Request, id int64, secret string) bool {
	c, err := r.Cookie(pinCookie(id))
	return err == nil && secret != "" && subtle.ConstantTimeCompare([]byte(c.Value), []byte(secret)) == 1
}

func (a *Auth) mediaServerLogin(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	v, ok := a.mediaWay(w, r, kind)
	if !ok {
		return
	}
	got, pl, ok := a.checkMediaServer(w, r, kind, v)
	if !ok {
		return
	}
	id, name, err := a.linkedUser(r.Context(), kind, got.ID, got.Name, allowed(v, got.Name), got.Limited)
	a.outsideDone(w, r, kind, id, name, err, pl)
}

// mediaWay is the Jellyfin or Emby sign-in way, or false after answering
// 404 when there is no such way or it is off.
func (a *Auth) mediaWay(w http.ResponseWriter, r *http.Request, kind string) (WayView, bool) {
	if kind != ProviderJellyfin && kind != ProviderEmby {
		fail(w, http.StatusNotFound, "no such sign-in way")
		return WayView{}, false
	}
	v := a.way(r.Context(), kind)
	if !v.On || v.URL == "" {
		fail(w, http.StatusNotFound, "sign-in with "+kind+" is off")
		return v, false
	}
	return v, true
}

// mediaMe is the Jellyfin or Emby account a name and password belong to.
type mediaMe struct {
	ID, Name string
	// Limited is set when the server limits what this person may watch.
	Limited bool
}

// checkMediaServer checks the name and password in the request against the
// owner's Jellyfin or Emby server, with the sign-in limits. It returns the
// account and the place to record, or false after answering.
func (a *Auth) checkMediaServer(w http.ResponseWriter, r *http.Request, kind string, v WayView) (mediaMe, place, bool) {
	ctx := r.Context()
	c, ok := readCreds(w, r)
	if !ok {
		return mediaMe{}, place{}, false
	}
	if c.Name == "" || len(c.Name) > 128 || len(c.Password) > MaxPassword {
		fail(w, http.StatusBadRequest, "give your "+kind+" name and password")
		return mediaMe{}, place{}, false
	}
	ipKey, altKey, sure := a.clientKeys(r)
	userKey := "name:" + kind + ":" + strings.ToLower(c.Name)
	var marks []failMark
	a.mu.Lock()
	wait := max(a.signInWait(ipKey, userKey, sure), a.altWait(altKey))
	if wait == 0 {
		marks = a.markFails(ipKey, altKey, userKey)
		a.noteFail(ipKey, altKey, userKey)
	}
	a.mu.Unlock()
	// Only an attempt the server could not check is taken back: no answer
	// at all, or a proxy saying the server behind it is down (502, 503,
	// 504). Any other answer counts, so a server that turns down wrong
	// passwords with an odd status is still limited. A client that hung up
	// mid-check keeps its count, so hanging up skips no wait.
	undo := func(code int) {
		down := code == 0 || code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
		if down && ctx.Err() == nil {
			a.undoFail(marks)
		}
	}
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many failed sign-ins; wait a little and try again")
		return mediaMe{}, place{}, false
	}
	authz := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="1"`, product, product, a.clientID(ctx))
	hdr := map[string]string{"Authorization": authz, "X-Emby-Authorization": authz}
	var got struct {
		User struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Policy struct {
				IsDisabled bool `json:"IsDisabled"`
				// Set when the server limits what this person may watch.
				MaxParentalRating *int `json:"MaxParentalRating"`
			} `json:"Policy"`
		} `json:"User"`
		AccessToken string `json:"AccessToken"`
	}
	code, err := a.serverFetch(ctx, http.MethodPost, v.URL+"/Users/AuthenticateByName", hdr, map[string]string{"Username": c.Name, "Pw": c.Password}, &got)
	if err != nil {
		undo(code)
		log.Printf("auth: %s sign-in: %v", kind, err)
		fail(w, http.StatusBadGateway, "the "+kind+" server did not answer; ask the owner")
		return mediaMe{}, place{}, false
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden || code/100 == 4 {
		log.Printf("auth: failed %s sign-in from %s", kind, r.RemoteAddr)
		a.failedEvent(r, a.linkedName(ctx, kind, "", c.Name), kind)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return mediaMe{}, place{}, false
	}
	if code/100 != 2 || got.User.ID == "" {
		undo(code)
		log.Printf("auth: %s sign-in: status %d", kind, code)
		fail(w, http.StatusBadGateway, "the "+kind+" server gave an odd answer; ask the owner")
		return mediaMe{}, place{}, false
	}
	if got.AccessToken != "" {
		// HolyArrs does not keep the server's token: end that session now.
		lh := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, got.AccessToken)}
		lh["X-Emby-Token"] = got.AccessToken
		_, _ = a.serverFetch(context.WithoutCancel(ctx), http.MethodPost, v.URL+"/Sessions/Logout", lh, nil, nil)
	}
	if got.User.Policy.IsDisabled {
		a.failedEvent(r, a.linkedName(ctx, kind, got.User.ID, got.User.Name), kind+" turned off")
		fail(w, http.StatusForbidden, "this account is turned off on the "+kind+" server")
		return mediaMe{}, place{}, false
	}
	a.mu.Lock()
	a.undoFailLocked(marks[:2]) // this attempt's address counts only (ipKey, altKey)
	delete(a.fails, userKey)
	a.mu.Unlock()
	return mediaMe{ID: got.User.ID, Name: got.User.Name, Limited: got.User.Policy.MaxParentalRating != nil}, place{ipKey, userKey, sure}, true
}
