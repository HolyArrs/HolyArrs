package auth

import (
	"context"
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
	if a.sso != nil && a.sso.oidc != nil {
		out = append(out, "oidc")
	}
	if a.sso != nil && a.sso.forward != nil {
		out = append(out, "forward_auth")
	}
	return out
}

func (a *Auth) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, http.StatusOK, map[string]any{
		settingSkipLocal:   a.skipLocal.Load(),
		keyRequireAdmin2FA: a.requireAdmin2FA.Load(),
		keyMinLength:       MinPasswordLength(ctx, a.s.Read()),
		ProviderPlex:       a.way(ctx, ProviderPlex),
		ProviderJellyfin:   a.way(ctx, ProviderJellyfin),
		ProviderEmby:       a.way(ctx, ProviderEmby),
	})
}

func (a *Auth) putSettings(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	var body struct {
		SkipLocal    *bool    `json:"skip_login_local_network"`
		RequireAdmin *bool    `json:"require_two_factor_admins"`
		MinPassword  *int     `json:"min_password_length"`
		Plex         *WayView `json:"plex"`
		Jellyfin     *WayView `json:"jellyfin"`
		Emby         *WayView `json:"emby"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil ||
		(body.SkipLocal == nil && body.RequireAdmin == nil && body.MinPassword == nil && body.Plex == nil && body.Jellyfin == nil && body.Emby == nil) {
		fail(w, http.StatusBadRequest, `send {"skip_login_local_network": true|false, "require_two_factor_admins": true|false, "min_password_length": 4..64, "plex": {...}, "jellyfin": {...}, "emby": {...}}`)
		return
	}
	set := map[string]string{}
	if body.SkipLocal != nil {
		set["this_app/"+settingSkipLocal] = strconv.FormatBool(*body.SkipLocal)
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

// linkedUser finds or makes the user for an outside account.
func (a *Auth) linkedUser(ctx context.Context, provider, extID, extName string, mayJoin bool) (int64, string, error) {
	var id int64
	var name, state string
	err := a.s.Read().QueryRowContext(ctx, `SELECT u.id, u.name, u.state FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_id = ?`, provider, extID).Scan(&id, &name, &state)
	if err == nil {
		if state != "active" {
			return 0, "", errDisabled
		}
		return id, name, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}
	if !mayJoin {
		return 0, "", errNotAllowed
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
		id, err = store.Insert(tx, "users", &domain.User{Name: name, Role: RoleMember, State: "active",
			Perms: permsJSON(DefaultPerms(RoleMember))}, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?, ?, ?, ?, ?)",
			id, provider, extID, truncate(extName, 200), store.FormatTime(a.now()))
		return err
	})
	if err == nil {
		log.Printf("auth: %s account joined as member %q (%d)", provider, name, id)
	}
	return id, name, err
}

// outsideDone answers after an outside account checked out.
func (a *Auth) outsideDone(w http.ResponseWriter, r *http.Request, provider string, id int64, name string, err error) {
	switch {
	case errors.Is(err, errNotAllowed):
		log.Printf("auth: %s account not allowed to join, from %s", provider, r.RemoteAddr)
		fail(w, http.StatusForbidden, "this account is not allowed in yet; ask the owner")
	case errors.Is(err, errDisabled):
		fail(w, http.StatusForbidden, "this account is turned off; ask the owner")
	case err != nil:
		serverError(w, provider+" sign-in", err)
	default:
		a.passedAs(w, r, id, name, provider)
	}
}

// fetch makes one call to an outside service and decodes a JSON answer.
func (a *Auth) fetch(ctx context.Context, method, u string, hdr map[string]string, body any, out any) (int, error) {
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
	resp, err := a.http.Do(req)
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
	ip      string
	expires time.Time
}

func (a *Auth) plexHeaders(ctx context.Context) map[string]string {
	return map[string]string{"X-Plex-Product": product, "X-Plex-Client-Identifier": a.clientID(ctx)}
}

func (a *Auth) plexStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !a.way(ctx, ProviderPlex).On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
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
	a.mu.Lock()
	a.pins[got.ID] = &pin{ip: ip, expires: now.Add(pinLife)}
	a.mu.Unlock()
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
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"id": ...}`)
		return
	}
	a.mu.Lock()
	p := a.pins[body.ID]
	ok := p != nil && p.ip == a.clientKey(r) && a.now().Before(p.expires)
	a.mu.Unlock()
	if !ok {
		fail(w, http.StatusNotFound, "this Plex sign-in has ended; start again")
		return
	}
	var pinState struct {
		AuthToken string `json:"authToken"`
	}
	hdr := a.plexHeaders(ctx)
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/pins/"+strconv.FormatInt(body.ID, 10), hdr, nil, &pinState)
	if err != nil || code/100 != 2 {
		log.Printf("auth: plex pin check: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not answer; try again in a moment")
		return
	}
	if pinState.AuthToken == "" {
		writeJSON(w, http.StatusAccepted, map[string]bool{"waiting": true})
		return
	}
	a.mu.Lock()
	delete(a.pins, body.ID) // one pin, one sign-in
	a.mu.Unlock()
	hdr["X-Plex-Token"] = pinState.AuthToken
	var user struct {
		ID       int64  `json:"id"`
		UUID     string `json:"uuid"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	code, err = a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/user", hdr, nil, &user)
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
		}) bool {
			return x.ClientIdentifier == v.ServerID
		})
	}
	id, name, err := a.linkedUser(ctx, ProviderPlex, extID, user.Username, mayJoin)
	a.outsideDone(w, r, ProviderPlex, id, name, err)
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
	a.mu.Lock()
	wait := max(a.waitFor(ipKey, freeTries), a.waitFor(userKey, freeUserTries))
	if wait == 0 {
		a.noteFail(ipKey, userKey)
	}
	a.mu.Unlock()
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
			} `json:"Policy"`
		} `json:"User"`
		AccessToken string `json:"AccessToken"`
	}
	code, err := a.fetch(ctx, http.MethodPost, v.URL+"/Users/AuthenticateByName", hdr, map[string]string{"Username": c.Name, "Pw": c.Password}, &got)
	if err != nil {
		log.Printf("auth: %s sign-in: %v", kind, err)
		fail(w, http.StatusBadGateway, "the "+kind+" server did not answer; ask the owner")
		return
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden || code/100 == 4 {
		log.Printf("auth: failed %s sign-in from %s", kind, r.RemoteAddr)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	if code/100 != 2 || got.User.ID == "" {
		log.Printf("auth: %s sign-in: status %d", kind, code)
		fail(w, http.StatusBadGateway, "the "+kind+" server gave an odd answer; ask the owner")
		return
	}
	if got.AccessToken != "" {
		// HolyArrs does not keep the server's token: end that session now.
		lh := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, got.AccessToken)}
		lh["X-Emby-Token"] = got.AccessToken
		_, _ = a.fetch(context.WithoutCancel(ctx), http.MethodPost, v.URL+"/Sessions/Logout", lh, nil, nil)
	}
	if got.User.Policy.IsDisabled {
		fail(w, http.StatusForbidden, "this account is turned off on the "+kind+" server")
		return
	}
	a.mu.Lock()
	delete(a.fails, ipKey)
	delete(a.fails, userKey)
	a.mu.Unlock()
	id, name, err := a.linkedUser(ctx, kind, got.User.ID, got.User.Name, allowed(v, got.User.Name))
	a.outsideDone(w, r, kind, id, name, err)
}
