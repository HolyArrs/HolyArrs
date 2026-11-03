package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
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
	maxPins    = 200
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
		On:       a.signInSetting(ctx, p+"_on") == "true",
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
		_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES (?, ?, ?, ?, '{"kind":"system"}', ?, '{"kind":"system"}') ON CONFLICT(grp, key) DO NOTHING`,
			settingsGroup, signInPrefix+keyClientID, id, store.FormatTime(a.now()), store.FormatTime(a.now()))
		return err
	})
	return a.signInSetting(ctx, keyClientID)
}

func (a *Auth) outsideRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/plex/start", a.plexStart)
	m.HandleFunc("POST /api/v1/auth/plex/finish", a.plexFinish)
	m.HandleFunc("POST /api/v1/auth/media-server/{kind}", a.mediaServerLogin)
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
	writeJSON(w, http.StatusOK, map[string]any{
		settingSkipLocal:   a.skipLocal.Load(),
		keyLocalNets:       nets,
		keyRequireAdmin2FA: a.requireAdmin2FA.Load(),
		keyMinLength:       MinPasswordLength(ctx, a.s.Read()),
		ProviderPlex:       a.way(ctx, ProviderPlex),
		ProviderJellyfin:   a.way(ctx, ProviderJellyfin),
		ProviderEmby:       a.way(ctx, ProviderEmby),
		"plex_owner":       a.plexOwner(ctx),
	})
}

func (a *Auth) putSettings(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
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
		// ConfirmEveryone must be true to open a way to everyone the
		// server lets in, when it was not open to everyone already.
		ConfirmEveryone bool `json:"confirm_everyone"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil ||
		(body.SkipLocal == nil && body.LocalNets == nil && body.RequireAdmin == nil && body.MinPassword == nil && body.Plex == nil && body.Jellyfin == nil && body.Emby == nil) {
		fail(w, http.StatusBadRequest, `send {"skip_login_local_network": true|false, "local_networks": ["192.168.1.0/24"], "require_two_factor_admins": true|false, "min_password_length": 4..64, "plex": {...}, "jellyfin": {...}, "emby": {...}}`)
		return
	}
	if body.SkipLocal != nil && *body.SkipLocal {
		var kids int
		if err := a.s.Read().QueryRowContext(r.Context(), "SELECT count(*) FROM users WHERE max_rating != ''").Scan(&kids); err != nil {
			serverError(w, "sign-in settings", err)
			return
		}
		if kids > 0 {
			fail(w, http.StatusBadRequest, "Skipping sign-in on the local network cannot be on while a Kid account has an age limit: anyone at home would sign in as the owner. Remove the limits first.")
			return
		}
		// An open invite with an age limit makes a Kid once it is used.
		rows, err := a.s.Read().QueryContext(r.Context(), "SELECT expires_at FROM invites WHERE used_at IS NULL AND max_rating != ''")
		if err != nil {
			serverError(w, "sign-in settings", err)
			return
		}
		open := false
		for rows.Next() {
			var expires string
			if err := rows.Scan(&expires); err != nil {
				rows.Close()
				serverError(w, "sign-in settings", err)
				return
			}
			if a.now().Before(parseTime(expires)) {
				open = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			serverError(w, "sign-in settings", err)
			return
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
	by := Actor(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		for k, v := range set {
			grp, key, _ := strings.Cut(k, "/")
			if err := putSetting(tx, grp, key, v, by, a.now()); err != nil {
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
	var id int64
	var name, state string
	err := a.s.Read().QueryRowContext(ctx, `SELECT u.id, u.name, u.state FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_id = ?`, provider, extID).Scan(&id, &name, &state)
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
	u := domain.User{Role: RoleMember, State: "active", Perms: permsJSON(DefaultPerms(RoleMember))}
	if kid {
		if a.skipLocal.Load() {
			// A Kid could not be told apart from the owner at home.
			return 0, "", errKidSkipLocal
		}
		u.Perms, u.MaxRating = permsJSON([]Perm{PermAsk}), "PG"
	}
	base := strings.TrimSpace(extName)
	if base == "" {
		base = provider + "-user"
	}
	base = truncate(base, 56)
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		name = base
		for i := 2; ; i++ {
			var n int
			if err := tx.QueryRow("SELECT count(*) FROM users WHERE name = ?", name).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				break
			}
			name = fmt.Sprintf("%s-%d", base, i)
		}
		var err error
		u.Name = name
		id, err = store.Insert(tx, "users", &u, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?, ?, ?, ?, ?)",
			id, provider, extID, truncate(extName, 200), store.FormatTime(a.now()))
		return err
	})
	if err == nil {
		log.Printf("auth: %s account joined as member %q (%d), kid %t", provider, name, id, kid)
	}
	return id, name, err
}

// linkedName is the member an outside account belongs to, found through its
// link (by id, else by its name on that server), else the member with that
// name, else unknownName. The typed name is only matched, never stored.
func (a *Auth) linkedName(ctx context.Context, provider, extID, extName string) string {
	var n string
	if extID != "" && a.s.Read().QueryRowContext(ctx, `SELECT u.name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_id = ?`, provider, extID).Scan(&n) == nil {
		return n
	}
	if a.s.Read().QueryRowContext(ctx, `SELECT u.name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_name = ? ORDER BY l.user_id LIMIT 1`, provider, extName).Scan(&n) == nil {
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
func (a *Auth) outsideDone(w http.ResponseWriter, r *http.Request, provider string, id int64, name string, err error) {
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
		a.passedAs(w, r, id, name, provider)
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
	// secret is also in a cookie on the browser that started the pin, so
	// another visitor behind the same address cannot finish it.
	secret  string
	expires time.Time
	// checked is the last time Plex was asked about this pin; checks
	// closer than pinCheckGap answer 429 (the page polls every 2 s).
	checked time.Time
	link    bool // the owner linking their account in Settings, not a sign-in
}

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
	a.startPin(w, r, false)
}

// startPin asks Plex for a PIN and answers with the page to confirm it on.
func (a *Auth) startPin(w http.ResponseWriter, r *http.Request, link bool) {
	ctx := r.Context()
	ip := a.clientKey(r)
	now := a.now()
	a.mu.Lock()
	mine := 0
	for id, p := range a.pins {
		if now.After(p.expires) {
			delete(a.pins, id)
		} else if p.ip == ip {
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
	a.pins[got.ID] = &pin{ip: ip, secret: secret, expires: now.Add(pinLife), link: link}
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
	tok, ok := a.checkPin(w, r, false)
	if !ok {
		return
	}
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var user struct {
		ID       int64  `json:"id"`
		UUID     string `json:"uuid"`
		Username string `json:"username"`
		Email    string `json:"email"`
		// A managed Plex Home user with limits on what they may watch.
		Restricted bool `json:"restricted"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/user", hdr, nil, &user)
	if err != nil || code/100 != 2 || user.ID == 0 {
		log.Printf("auth: plex user: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not say who you are; try again")
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
	a.outsideDone(w, r, ProviderPlex, id, name, err)
}

// checkPin asks Plex whether the person confirmed the PIN. It returns the
// token once they did; otherwise it has answered (waiting, ended, too soon).
// A PIN started to link the owner's account cannot sign anyone in, and the
// other way round.
func (a *Auth) checkPin(w http.ResponseWriter, r *http.Request, link bool) (string, bool) {
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
	ok := p != nil && p.link == link && p.ip == a.clientKey(r) && a.now().Before(p.expires) && pinSecretMatches(r, body.ID, p.secret)
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
	ctx := r.Context()
	kind := r.PathValue("kind")
	if kind != ProviderJellyfin && kind != ProviderEmby {
		fail(w, http.StatusNotFound, "no such sign-in way")
		return
	}
	v := a.way(ctx, kind)
	if !v.On || v.URL == "" {
		fail(w, http.StatusNotFound, "sign-in with "+kind+" is off")
		return
	}
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if c.Name == "" || len(c.Name) > 128 || len(c.Password) > MaxPassword {
		fail(w, http.StatusBadRequest, "give your "+kind+" name and password")
		return
	}
	ipKey, userKey := a.clientKey(r), "name:"+kind+":"+strings.ToLower(c.Name)
	var marks []failMark
	a.mu.Lock()
	wait := max(a.waitFor(ipKey, freeTries), a.waitFor(userKey, freeUserTries))
	if wait == 0 {
		marks = a.markFails(ipKey, userKey)
		a.noteFail(ipKey, userKey)
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
		return
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
		return
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden || code/100 == 4 {
		log.Printf("auth: failed %s sign-in from %s", kind, r.RemoteAddr)
		a.failedEvent(r, a.linkedName(ctx, kind, "", c.Name), kind)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	if code/100 != 2 || got.User.ID == "" {
		undo(code)
		log.Printf("auth: %s sign-in: status %d", kind, code)
		fail(w, http.StatusBadGateway, "the "+kind+" server gave an odd answer; ask the owner")
		return
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
		return
	}
	a.mu.Lock()
	delete(a.fails, ipKey)
	delete(a.fails, userKey)
	a.mu.Unlock()
	id, name, err := a.linkedUser(ctx, kind, got.User.ID, got.User.Name, allowed(v, got.User.Name), got.User.Policy.MaxParentalRating != nil)
	a.outsideDone(w, r, kind, id, name, err)
}
