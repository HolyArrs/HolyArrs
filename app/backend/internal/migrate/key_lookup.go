package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// keyLookupWait bounds each ask to the app: it is a shortcut, never a wait.
const keyLookupWait = 3 * time.Second

// keyLookupTotal bounds a whole fetch, sign-in included.
const keyLookupTotal = 8 * time.Second

var arrKeyShape = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// seerrKeyShape is Seerr's and Overseerr's key: a base64-like token.
var seerrKeyShape = regexp.MustCompile(`^[A-Za-z0-9+/=_.\-]{1,200}$`)

// bazarrKeyIn finds the key Bazarr writes into its start page for its own
// web app ("apiKey": "...") when its sign-in is off or the caller signed in.
var bazarrKeyIn = regexp.MustCompile(`"apiKey"\s*:\s*"([0-9a-fA-F]{32})"`)

// Why a fetch found no key, for the setup screen.
const (
	keyWrongLogin  = "wrong_login"
	keyNotAdmin    = "not_admin"
	keyUnreachable = "unreachable"
)

// KeyLogin is a sign-in to the old app, used once for one fetch and never kept.
type KeyLogin struct {
	Username, Password string
	// PlexToken signs in to Seerr or Overseerr with Plex instead.
	PlexToken string
	// MediaServer signs in to Seerr with a Jellyfin or Emby account (Username, Password).
	MediaServer bool
}

func (l KeyLogin) given() bool { return l.Username != "" && l.Password != "" || l.PlexToken != "" }

// KeyResult is what a fetch found: the key, or why there is none.
type KeyResult struct {
	Key string
	// NeedLogin says the app refused without a sign-in and none was given.
	NeedLogin bool
	// Problem is keyWrongLogin, keyNotAdmin, keyUnreachable or "".
	Problem string
	// MediaServer is the Jellyfin or Emby a Seerr signs people in with ("jellyfin", "emby" or "").
	MediaServer string
}

// LookupKey asks a Radarr, Sonarr, Lidarr or Prowlarr for its startup file
// (/initialize.json), or Bazarr for its start page, which hand out the API
// key when that app's sign-in is off. Any failure answers "".
func LookupKey(ctx context.Context, kind Kind, address string, allowLocal bool) string {
	return LookupKeyWith(ctx, kind, address, allowLocal, KeyLogin{}).Key
}

// keyAsker makes requests to one app: no redirects followed, one cookie jar,
// small bodies.
type keyAsker struct {
	c    *http.Client
	base url.URL
}

func newKeyAsker(address string, allowLocal bool) (*keyAsker, bool) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(address), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return nil, false
	}
	c := downloads.Client(u.String(), allowLocal)
	c.Timeout = keyLookupWait
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// The app's own cookies for this one lookup, kept even when marked Secure on a
	// plain-http home address (Overseerr with CSRF protection marks them so).
	c.Jar = &hostJar{host: u.Host}
	u.Path = strings.TrimRight(u.Path, "/")
	return &keyAsker{c: c, base: *u}, true
}

// do sends one request to path under the app's address and reads at most
// limit bytes of the answer. A failure to reach the app gives status 0.
func (k *keyAsker) do(ctx context.Context, method, path, ctype string, body []byte, auth func(*http.Request), limit int64) (int, []byte) {
	u := k.base
	p, q, _ := strings.Cut(path, "?")
	u.Path += p
	u.RawQuery = q
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return 0, nil
	}
	req.Header.Set("Accept", "application/json, text/html")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if auth != nil {
		auth(req)
	}
	resp, err := k.c.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	return resp.StatusCode, b
}

// LookupKeyWith fetches the app's key, signing in with login when the app
// asks for it. The login is used for this one fetch and never kept or logged.
func LookupKeyWith(ctx context.Context, kind Kind, address string, allowLocal bool, login KeyLogin) KeyResult {
	k, ok := newKeyAsker(address, allowLocal)
	if !ok {
		return KeyResult{}
	}
	switch kind {
	case Radarr, Sonarr, Lidarr, Prowlarr:
		return k.arrKey(ctx, login)
	case Bazarr:
		return k.bazarrKey(ctx, login)
	case Seerr, Overseerr:
		return k.seerrKey(ctx, login)
	case Jackett:
		return k.jackettKey(ctx)
	case SABnzbd:
		return k.sabKey(ctx, login)
	}
	return KeyResult{}
}

func arrKeyIn(status int, body []byte) string {
	if status != http.StatusOK {
		return ""
	}
	var v struct {
		APIKey string `json:"apiKey"`
	}
	if json.Unmarshal(body, &v) != nil || !arrKeyShape.MatchString(v.APIKey) {
		return ""
	}
	return v.APIKey
}

// refused says the app wants a sign-in: 401, 403, or a redirect (to its login page).
func refused(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status >= 300 && status < 400
}

func basicAuth(l KeyLogin) func(*http.Request) {
	return func(r *http.Request) { r.SetBasicAuth(l.Username, l.Password) }
}

func loginForm(l KeyLogin, extra ...string) []byte {
	v := url.Values{"username": {l.Username}, "password": {l.Password}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return []byte(v.Encode())
}

const formType = "application/x-www-form-urlencoded"

// arrKey reads /initialize.json; with a login it tries Basic, then the app's
// own sign-in form (POST /login, session cookie).
func (k *keyAsker) arrKey(ctx context.Context, login KeyLogin) KeyResult {
	st, b := k.do(ctx, http.MethodGet, "/initialize.json", "", nil, nil, 64<<10)
	if key := arrKeyIn(st, b); key != "" {
		return KeyResult{Key: key}
	}
	if st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	if !refused(st) {
		return KeyResult{}
	}
	if login.Username == "" || login.Password == "" {
		return KeyResult{NeedLogin: true}
	}
	if key := arrKeyIn(k.do(ctx, http.MethodGet, "/initialize.json", "", nil, basicAuth(login), 64<<10)); key != "" {
		return KeyResult{Key: key}
	}
	if st, _ := k.do(ctx, http.MethodPost, "/login", formType, loginForm(login, "rememberMe", "on"), nil, 16<<10); st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	if key := arrKeyIn(k.do(ctx, http.MethodGet, "/initialize.json", "", nil, nil, 64<<10)); key != "" {
		return KeyResult{Key: key}
	}
	return KeyResult{Problem: keyWrongLogin}
}

func bazarrKeyFrom(status int, page []byte) string {
	if status != http.StatusOK {
		return ""
	}
	if m := bazarrKeyIn.FindSubmatch(page); m != nil {
		return string(m[1])
	}
	return ""
}

// bazarrKey reads the start page; with a login it tries Basic, then Bazarr's
// own sign-in (POST /api/system/account?action=login).
func (k *keyAsker) bazarrKey(ctx context.Context, login KeyLogin) KeyResult {
	st, b := k.do(ctx, http.MethodGet, "/", "", nil, nil, 256<<10)
	if key := bazarrKeyFrom(st, b); key != "" {
		return KeyResult{Key: key}
	}
	if st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	if st != http.StatusOK && !refused(st) {
		return KeyResult{}
	}
	if login.Username == "" || login.Password == "" {
		return KeyResult{NeedLogin: true}
	}
	if key := bazarrKeyFrom(k.do(ctx, http.MethodGet, "/", "", nil, basicAuth(login), 256<<10)); key != "" {
		return KeyResult{Key: key}
	}
	if st, _ := k.do(ctx, http.MethodPost, "/api/system/account?action=login", formType, loginForm(login), nil, 16<<10); st == 0 {
		return KeyResult{Problem: keyUnreachable}
	}
	if key := bazarrKeyFrom(k.do(ctx, http.MethodGet, "/", "", nil, nil, 256<<10)); key != "" {
		return KeyResult{Key: key}
	}
	return KeyResult{Problem: keyWrongLogin}
}

// seerrKey signs in to Seerr or Overseerr (with email and password, or the
// owner's Plex token) and reads the key from the main settings, which only an
// admin may see. Seerr never hands its key out without a sign-in.
func (k *keyAsker) seerrKey(ctx context.Context, login KeyLogin) KeyResult {
	if !login.given() {
		return KeyResult{NeedLogin: true, MediaServer: k.seerrMediaServer(ctx)}
	}
	var body []byte
	path := "/api/v1/auth/local"
	switch {
	case login.MediaServer:
		path = "/api/v1/auth/jellyfin" // Seerr signs in Jellyfin and Emby accounts here
		body, _ = json.Marshal(map[string]string{"username": login.Username, "password": login.Password})
	case login.PlexToken != "":
		path = "/api/v1/auth/plex"
		body, _ = json.Marshal(map[string]string{"authToken": login.PlexToken})
	default:
		body, _ = json.Marshal(map[string]string{"email": login.Username, "password": login.Password})
	}
	// With CSRF protection on, the app hands out its token on any page first
	// and wants it back in a header on the sign-in.
	k.do(ctx, http.MethodGet, "/api/v1/settings/public", "", nil, nil, 16<<10)
	var echo func(*http.Request)
	if j, ok := k.c.Jar.(*hostJar); ok {
		if tok := j.xsrf(); tok != "" {
			echo = func(r *http.Request) { r.Header.Set("X-XSRF-TOKEN", tok); r.Header.Set("CSRF-Token", tok) }
		}
	}
	st, rb := k.do(ctx, http.MethodPost, path, "application/json", body, echo, 16<<10)
	switch {
	case st == 0:
		return KeyResult{Problem: keyUnreachable}
	case st != http.StatusOK:
		// Why the app refused, in its own words (its error field only: never the
		// login, token or key we sent).
		log.Printf("key lookup: %s refused the sign-in at %s: %d %s", k.base.Host, path, st, appReason(rb))
		return KeyResult{Problem: keyWrongLogin}
	}
	st, b := k.do(ctx, http.MethodGet, "/api/v1/settings/main", "", nil, nil, 64<<10)
	switch {
	case st == 0:
		return KeyResult{Problem: keyUnreachable}
	case st == http.StatusForbidden || st == http.StatusUnauthorized:
		return KeyResult{Problem: keyNotAdmin}
	case st != http.StatusOK:
		return KeyResult{}
	}
	var v struct {
		APIKey string `json:"apiKey"`
	}
	if json.Unmarshal(b, &v) != nil || !seerrKeyShape.MatchString(v.APIKey) {
		return KeyResult{}
	}
	return KeyResult{Key: v.APIKey}
}

// seerrMediaServer reads which Jellyfin or Emby a Seerr signs people in
// with from its public settings (mediaServerType 2 Jellyfin, 3 Emby).
func (k *keyAsker) seerrMediaServer(ctx context.Context) string {
	st, b := k.do(ctx, http.MethodGet, "/api/v1/settings/public", "", nil, nil, 64<<10)
	var v struct {
		MediaServerType int `json:"mediaServerType"`
	}
	if st != http.StatusOK || json.Unmarshal(b, &v) != nil {
		return ""
	}
	switch v.MediaServerType {
	case 2:
		return "jellyfin"
	case 3:
		return "emby"
	}
	return ""
}

// keyLookup answers {key, need_login, plex_available, problem} for the setup
// screen; the key and the login are never logged.
func (h *Handlers) keyLookup(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in struct {
		Kind       Kind   `json:"kind"`
		URL        string `json:"url"`
		AllowLocal bool   `json:"allow_local_network"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		UsePlex    bool   `json:"use_plex"`
		// UseMediaServer signs in to Seerr with a Jellyfin or Emby account.
		UseMediaServer bool `json:"use_media_server"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || in.URL == "" {
		webserver.Fail(w, http.StatusBadRequest, "the body must be JSON naming the app and an address or a backup path")
		return
	}
	// The stored Plex token goes to whatever address was typed, so only a
	// signed-in browser may send it, never an API key.
	if who, ok := webserver.From(r.Context()); ok && in.UsePlex && who.Via != "session" {
		webserver.FailNoStore(w, http.StatusForbidden, "sign in with your password to use the Plex sign-in")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), keyLookupTotal)
	defer cancel()
	seerr := in.Kind == Seerr || in.Kind == Overseerr
	plex := ""
	if seerr && h.PlexToken != nil {
		plex = h.PlexToken(r.Context())
	}
	login := KeyLogin{Username: in.Username, Password: in.Password}
	switch {
	case in.UsePlex && seerr:
		login = KeyLogin{PlexToken: plex}
	case in.UseMediaServer && in.Kind == Seerr:
		login.MediaServer = true
	}
	res := LookupKeyWith(ctx, in.Kind, in.URL, in.AllowLocal, login)
	out := struct {
		Key           string `json:"key"`
		NeedLogin     bool   `json:"need_login,omitempty"`
		PlexAvailable bool   `json:"plex_available,omitempty"`
		Problem       string `json:"problem,omitempty"`
		MediaServer   string `json:"media_server,omitempty"`
	}{Key: res.Key, NeedLogin: res.NeedLogin, PlexAvailable: plex != "", Problem: res.Problem}
	if in.Kind == Seerr {
		out.MediaServer = res.MediaServer
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// hostJar keeps the cookies one app sets during a single key lookup and sends
// them back to that app, ignoring the Secure flag: the lookup only ever talks
// to the one address the owner typed, and plain http is normal at home.
type hostJar struct {
	mu   sync.Mutex
	host string // the one host:port cookies belong to; the first seen when empty
	c    map[string]*http.Cookie
}

// same reports whether u is the jar's one host, taking the first host seen
// when none was set. Cookies never go to, or come from, any other host.
func (j *hostJar) same(u *url.URL) bool {
	if u == nil {
		return false
	}
	if j.host == "" {
		j.host = u.Host
	}
	return strings.EqualFold(u.Host, j.host)
}

func (j *hostJar) SetCookies(u *url.URL, cs []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.same(u) {
		return
	}
	if j.c == nil {
		j.c = map[string]*http.Cookie{}
	}
	for _, c := range cs {
		if c.MaxAge < 0 {
			delete(j.c, c.Name)
			continue
		}
		j.c[c.Name] = &http.Cookie{Name: c.Name, Value: c.Value}
	}
}

func (j *hostJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.same(u) {
		return nil
	}
	out := make([]*http.Cookie, 0, len(j.c))
	for _, c := range j.c {
		out = append(out, c)
	}
	return out
}

// xsrf is the token Overseerr's CSRF protection wants echoed in a header.
func (j *hostJar) xsrf() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if c := j.c["XSRF-TOKEN"]; c != nil {
		return c.Value
	}
	return ""
}

// knownReasons are the app refusals worth logging as they are; anything else
// (which might echo the login typed) is logged as "refused".
var knownReasons = []string{
	"unauthorized", "forbidden", "invalid credentials", "access denied",
	"user not found", "incorrect password", "invalid password",
	"csrf", "too many requests", "not found",
}

// appReason is a short, safe reason for an app's error answer: one of
// knownReasons when its {"message"} or {"error"} names one, else "refused".
func appReason(b []byte) string {
	var v struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(b, &v)
	r := strings.ToLower(v.Message + " " + v.Error)
	for _, k := range knownReasons {
		if strings.Contains(r, k) {
			return k
		}
	}
	return "refused"
}
