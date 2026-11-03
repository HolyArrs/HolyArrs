// Package auth signs the owner in: the first-run setup, password sign-in with
// back-off, session cookies, CSRF checks, API keys for scripts and the
// "skip sign-in on the local network" choice (off by default).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/web"
)

// Names on the wire.
const (
	CookieName   = "holyarrs_session"
	CSRFHeader   = "X-CSRF-Token"
	APIKeyHeader = "X-Api-Key"
	keyPrefix    = "hak_"
)

// Session lifetimes.
const (
	IdleTimeout     = 7 * 24 * time.Hour
	AbsoluteTimeout = 30 * 24 * time.Hour
	RotateEvery     = time.Hour
	rotateGrace     = time.Minute // an old id keeps working this long, for requests already in flight
	touchEvery      = RotateEvery
)

// Back-off after failed sign-ins.
const (
	freeTries     = 5  // per client address
	freeUserTries = 20 // per name, so one address cannot lock the owner out quickly
	MaxBackoff    = 15 * time.Minute
)

const settingSkipLocal = "skip_login_local_network"

// Options tunes Auth.
type Options struct {
	Now func() time.Time
	// AllowedHosts are host names (no port) the app answers to besides IP
	// literals and localhost. Requests that change something, local-network
	// requests without sign-in, and the first-run setup need one of these in
	// Host, which stops DNS rebinding.
	AllowedHosts []string
	// TrustedProxies are proxy addresses or CIDR ranges whose forwarded
	// headers are believed. Empty by default.
	TrustedProxies []string
	// Vault keeps two-factor secrets encrypted. Without it two-factor
	// sign-in cannot be turned on.
	Vault *secrets.Vault
	// HTTP talks to Plex, Jellyfin and Emby; PlexURL and PlexAppURL point
	// at plex.tv and app.plex.tv unless a test changes them.
	HTTP       *http.Client
	PlexURL    string
	PlexAppURL string
	// SafeMode is told to the sign-in page and every signed-in page, which
	// show a banner.
	SafeMode bool
	SSO      SSOOptions
	fastHash bool
}

// EnvAllowedHosts and EnvTrustedProxies add comma-separated entries.
const (
	EnvAllowedHosts   = "HOLYARRS_ALLOWED_HOSTS"
	EnvTrustedProxies = "HOLYARRS_TRUSTED_PROXIES"
)

// EnvPlexURL and EnvPlexAppURL point Plex sign-in somewhere other than
// plex.tv and app.plex.tv; the browser tests use them for a fake Plex.
const (
	EnvPlexURL    = "HOLYARRS_PLEX_URL"
	EnvPlexAppURL = "HOLYARRS_PLEX_APP_URL"
)

// SetupCodeFile holds the one-time first-run code, inside the config folder.
const SetupCodeFile = "setup-code.txt"

// maxChecks caps password checks running at once.
const maxChecks = 4

// Auth guards every route.
type Auth struct {
	s               *store.Store
	now             func() time.Time
	hash            hashParams
	skipLocal       atomic.Bool
	dummy           string
	hosts           map[string]bool
	requireAdmin2FA atomic.Bool
	vault           *secrets.Vault
	http            *http.Client
	plexURL         string
	plexApp         string
	tickets         map[string]*ticket
	pins            map[int64]*pin
	offers          map[string]*importOffer // last import preview per provider
	tester          *http.Client            // address tests (providers.go)
	server          *http.Client            // Jellyfin/Emby calls; never follows redirects
	proxies         []*net.IPNet
	checks          chan struct{}
	safeMode        bool
	sso             *ssoState

	mu        sync.Mutex
	fails     map[string]*failure
	setupCode string
}

type failure struct {
	n    int
	last time.Time
}

// New makes Auth and loads its settings.
func New(s *store.Store, opt Options) *Auth {
	a := &Auth{s: s, now: opt.Now, hash: normalHash, fails: map[string]*failure{}, hosts: map[string]bool{"localhost": true},
		checks: make(chan struct{}, maxChecks)}
	if a.now == nil {
		a.now = time.Now
	}
	a.vault, a.http, a.plexURL, a.plexApp, a.safeMode = opt.Vault, opt.HTTP, opt.PlexURL, opt.PlexAppURL, opt.SafeMode
	a.tester = testClient()
	if a.http == nil {
		a.http = &http.Client{Timeout: 15 * time.Second}
	}
	server := *a.http
	server.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	a.server = &server
	if a.plexURL == "" {
		a.plexURL = os.Getenv(EnvPlexURL)
	}
	if a.plexApp == "" {
		a.plexApp = os.Getenv(EnvPlexAppURL)
	}
	if a.plexURL == "" {
		a.plexURL = "https://plex.tv"
	}
	if a.plexApp == "" {
		a.plexApp = "https://app.plex.tv"
	}
	a.tickets, a.pins = map[string]*ticket{}, map[int64]*pin{}
	split := func(v string) []string {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	for _, h := range append(opt.AllowedHosts, split(os.Getenv(EnvAllowedHosts))...) {
		a.hosts[strings.ToLower(strings.TrimSuffix(h, "."))] = true
	}
	for _, p := range append(opt.TrustedProxies, split(os.Getenv(EnvTrustedProxies))...) {
		if !strings.Contains(p, "/") {
			if ip := net.ParseIP(p); ip != nil && ip.To4() != nil {
				p += "/32"
			} else {
				p += "/128"
			}
		}
		if _, n, err := net.ParseCIDR(p); err == nil {
			a.proxies = append(a.proxies, n)
		} else {
			log.Printf("auth: ignoring trusted proxy %q: %v", p, err)
		}
	}
	if _, _, ok := a.owner(context.Background()); !ok {
		if err := a.makeSetupCode(); err != nil {
			log.Printf("auth: could not write the setup code: %v", err)
		}
	}
	if opt.fastHash {
		a.hash = testHash
	}
	a.dummy, _ = hashPassword("not a real password", a.hash)
	var v string
	err := s.Read().QueryRow("SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingSkipLocal).Scan(&v)
	if err == nil {
		a.skipLocal.Store(v == "true")
	}
	a.requireAdmin2FA.Store(a.signInSetting(context.Background(), keyRequireAdmin2FA) == "true")
	a.initSSO(opt.SSO)
	return a
}

type ctxKey struct{}

// Who is the signed-in person on a request.
type Who struct {
	UserID int64
	Name   string
	Via    string // session, api_key, local_network
	CSRF   string
	Role   string
	Perms  []Perm
	// TwoFactor says two-factor sign-in is on for this person.
	TwoFactor bool
	// MustTwoFactor is set for an admin without two-factor sign-in when the
	// owner requires it; they may only reach My account until they turn it on.
	MustTwoFactor bool
	// AuthMethod records how the browser session was established. LocalRecent
	// is true only for a non-future local proof less than ten minutes old.
	AuthMethod  string `json:"auth_method"`
	LocalRecent bool   `json:"local_recent"`
	// OutsideRecent is true for a Plex, Jellyfin or Emby sign-in less than
	// ten minutes old: the proof a person with no password yet can give.
	OutsideRecent bool `json:"outside_recent"`
	// Scopes are an API key's scopes (see Scopes); empty for
	// a session, which holds every scope.
	Scopes []string
	// MaxRating is a Kid's age-rating limit ("" for no limit) and
	// RatingCountry the country whose ratings apply (US). Both are read from
	// the users row on every request, so a changed limit holds at once.
	MaxRating     string
	RatingCountry string
	// KeyID and KeyLabel name the API key a request came with (0 and ""
	// for a session); AI connections write them into History.
	KeyID    int64
	KeyLabel string
}

// userCols are the users columns fill reads, prefixed with u.
const userCols = "u.id, u.name, u.role, u.perms, u.totp_secret_id IS NOT NULL, u.max_rating, u.rating_country"

// fill completes who from a row of userCols and the sign-in settings.
func (a *Auth) fill(who *Who, perms string) {
	who.Perms = parsePerms(perms)
	who.MustTwoFactor = who.Role == RoleAdmin && !who.TwoFactor && a.requireAdmin2FA.Load()
	who.RatingCountry = countryOr(who.RatingCountry)
}

// From returns who made the request, if anyone.
func From(ctx context.Context) (Who, bool) {
	w, ok := ctx.Value(ctxKey{}).(Who)
	return w, ok
}

// ContextWith puts who on ctx as the signed-in person (in-process callers
// and tests; requests get it from Middleware).
func ContextWith(ctx context.Context, who Who) context.Context {
	return context.WithValue(ctx, ctxKey{}, who)
}

// Actor is the request's actor for "who and when".
func Actor(ctx context.Context) domain.Actor {
	if w, ok := From(ctx); ok {
		return domain.Actor{Kind: "user", ID: w.UserID}
	}
	return domain.System
}

func token(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// public routes need no sign-in.
func public(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET /healthz", "HEAD /healthz", "GET /readyz", "HEAD /readyz", "GET /login", "GET /api/v1/auth/state",
		"POST /api/v1/auth/login", "POST /api/v1/auth/setup", "POST /api/v1/auth/login/two-factor",
		"POST /api/v1/auth/oidc/start", "GET /api/v1/auth/oidc/callback", "POST /api/v1/auth/oidc/finish",
		"POST /api/v1/auth/forward/login", "GET /api/v1/public/signin-art",
		"POST /api/v1/auth/plex/start", "POST /api/v1/auth/plex/finish",
		"POST /api/v1/auth/media-server/jellyfin", "POST /api/v1/auth/media-server/emby":
		return true
	}
	// The sign-in page's pictures (the handler serves only its own art).
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/public/signin-pic/t/p/") {
		return true
	}
	// Invite links: the page and the two calls it makes.
	if rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/auth/invites/"); ok && rest != "" && !strings.Contains(rest, "/") &&
		(r.Method == http.MethodGet || r.Method == http.MethodPost) {
		return true
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, "/invite/"); ok && rest != "" && !strings.Contains(rest, "/") && r.Method == http.MethodGet {
		return true
	}
	// The web app's own files hold no data; the sign-in page needs them.
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && web.IsAsset(r.URL.Path)
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// sameOrigin refuses a request a browser sent from another site. Requests
// without Origin and Sec-Fetch-Site come from scripts, not browsers.
func (a *Auth) sameOrigin(r *http.Request) bool {
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" && sfs != "same-origin" && sfs != "none" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if a.behindHTTPS(r) {
		scheme = "https"
	}
	if !strings.EqualFold(u.Scheme, scheme) {
		return false
	}
	originHost, ok := canonicalHostPort(u.Host, scheme)
	if !ok {
		return false
	}
	requestHost, ok := canonicalHostPort(r.Host, scheme)
	return ok && originHost == requestHost
}

func canonicalHostPort(authority, scheme string) (string, bool) {
	u, err := url.Parse("//" + authority)
	if err != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return "", false
	}
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", false
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", false
	}
	return net.JoinHostPort(host, strconv.Itoa(n)), true
}

// isLocal reports whether the peer address is on a private network or this
// machine.
func isLocal(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// hostAllowed checks the Host header: IP literals, localhost and the
// configured names pass. A rebinding page always carries its own domain name
// in Host, so it fails here.
func (a *Auth) hostAllowed(r *http.Request) bool {
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.ToLower(strings.TrimSuffix(strings.Trim(h, "[]"), "."))
	return h != "" && (net.ParseIP(h) != nil || a.hosts[h])
}

// guarded is what every state-changing browser request must pass.
func (a *Auth) guarded(r *http.Request) bool { return a.hostAllowed(r) && a.sameOrigin(r) }

func (a *Auth) trustedProxy(ip net.IP) bool {
	for _, n := range a.proxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// localPeer says whether the request comes from the local network. When a
// forwarding header is present the peer must be a trusted proxy, and then the
// address it forwards is the one checked.
func (a *Auth) localPeer(r *http.Request) bool {
	xff, fwd, real := r.Header.Get("X-Forwarded-For"), r.Header.Get("Forwarded"), r.Header.Get("X-Real-IP")
	if xff == "" && fwd == "" && real == "" {
		return isLocal(r.RemoteAddr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil || !a.trustedProxy(peer) || fwd != "" {
		return false
	}
	client := real
	if xff != "" {
		parts := strings.Split(xff, ",")
		client = strings.TrimSpace(parts[len(parts)-1])
	}
	return isLocal(client)
}

// Strong reports whether the request uses a browser session or an API key.
// It is kept for compatibility at handler boundaries; sensitive browser
// routes separately require RecentLocal in Gate. Requests let in by the
// local-network skip are not strong:
// they may not make API keys, change sign-in or security settings, read or
// replace secrets, or restore backups.
func Strong(ctx context.Context) bool {
	w, ok := From(ctx)
	return ok && (w.Via == "session" || w.Via == "api_key")
}

// RequireStrong writes 403 and returns false when the request is not Strong.
func RequireStrong(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := From(r.Context()); !ok {
		return true // no Auth in front (tests)
	}
	if !Strong(r.Context()) {
		fail(w, http.StatusForbidden, "sign in with your password for this")
		return false
	}
	return true
}

// behindHTTPS believes X-Forwarded-Proto only from a trusted proxy.
func (a *Auth) behindHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && a.trustedProxy(ip)
}

func wantsPage(r *http.Request) bool {
	return r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/live"
}

// Middleware lets only signed-in requests through.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public(r) {
			if unsafeMethod(r.Method) && !a.guarded(r) {
				fail(w, http.StatusForbidden, "request from another site or host name refused")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		k := r.Header.Get(APIKeyHeader)
		if k == "" && mcpPath(r.URL.Path) {
			// AI clients send their key as a bearer token.
			if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				k = strings.TrimSpace(v)
			}
		}
		if k != "" {
			who, ok := a.checkKey(r.Context(), k)
			if !ok {
				fail(w, http.StatusUnauthorized, "API key not accepted")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, who)))
			return
		}
		if who, ok := a.session(w, r); ok {
			if unsafeMethod(r.Method) && (!a.guarded(r) ||
				subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(who.CSRF)) != 1) {
				fail(w, http.StatusForbidden, "missing or wrong CSRF token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, who)))
			return
		}
		if a.skipLocal.Load() && a.hostAllowed(r) && a.localPeer(r) {
			if id, name, ok := a.owner(r.Context()); ok {
				if unsafeMethod(r.Method) && !a.guarded(r) {
					fail(w, http.StatusForbidden, "request from another site refused")
					return
				}
				who := Who{UserID: id, Name: name, Via: "local_network", Role: RoleOwner, Perms: DefaultPerms(RoleOwner)}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, who)))
				return
			}
		}
		if wantsPage(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		fail(w, http.StatusUnauthorized, "sign in first")
	})
}

// mcpPath is the AI connection route, the only one a bearer key reaches.
func mcpPath(p string) bool { return p == "/api/v1/mcp" || p == "/mcp" }

func (a *Auth) owner(ctx context.Context) (int64, string, bool) {
	var id int64
	var name string
	err := a.s.Read().QueryRowContext(ctx, "SELECT id, name FROM users WHERE role = 'owner' AND state = 'active'").Scan(&id, &name)
	return id, name, err == nil
}

// Routes adds the sign-in routes and the sign-in page.
func (a *Auth) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /login", web.Page)
	m.HandleFunc("GET /api/v1/auth/state", a.state)
	m.HandleFunc("POST /api/v1/auth/setup", a.setup)
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	m.HandleFunc("POST /api/v1/auth/logout", a.logout)
	m.HandleFunc("POST /api/v1/auth/reauth", a.reauth)
	m.HandleFunc("GET /api/v1/auth/me", a.me)
	m.HandleFunc("GET /api/v1/auth/keys", a.listKeys)
	m.HandleFunc("POST /api/v1/auth/keys", a.makeKey)
	m.HandleFunc("DELETE /api/v1/auth/keys/{id}", a.deleteKey)
	m.HandleFunc("GET /api/v1/auth/settings", a.getSettings)
	m.HandleFunc("PUT /api/v1/auth/settings", a.putSettings)
	a.twoFactorRoutes(m)
	a.userRoutes(m)
	a.avatarRoutes(m)
	a.sessionRoutes(m)
	a.outsideRoutes(m)
	a.providerRoutes(m)
	a.ssoRoutes(m)
}

func (a *Auth) state(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.owner(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": !ok, "ways": a.ways(r.Context()), "safe_mode": a.safeMode,
		"min_password_length": MinPasswordLength(r.Context(), a.s.Read())})
}

type creds struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"` // first-run setup code
}

// makeSetupCode makes the one-time first-run code, writes it to a 0600 file
// in the config folder and prints it to the log. An existing file is reused.
func (a *Auth) makeSetupCode() error {
	p := filepath.Join(a.s.Dir(), SetupCodeFile)
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) >= 16 {
		a.setupCode = strings.TrimSpace(string(b))
	} else {
		a.setupCode = token(15)
		if err := os.WriteFile(p, []byte(a.setupCode+"\n"), 0o600); err != nil {
			return err
		}
	}
	log.Printf("auth: first run: setup code %s (also in %s; it is deleted once the owner is made)", a.setupCode, p)
	return nil
}

func readCreds(w http.ResponseWriter, r *http.Request) (creds, bool) {
	var c creds
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&c); err != nil {
		fail(w, http.StatusBadRequest, `send {"name": "...", "password": "..."}`)
		return c, false
	}
	c.Name = strings.TrimSpace(c.Name)
	return c, true
}

func (a *Auth) setup(w http.ResponseWriter, r *http.Request) {
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if c.Name == "" || len(c.Name) > 64 {
		fail(w, http.StatusBadRequest, "pick a name of 1 to 64 characters")
		return
	}
	if msg := passwordProblem(c.Password, MinPasswordLength(r.Context(), a.s.Read())); msg != "" {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if _, _, exists := a.owner(r.Context()); exists {
		fail(w, http.StatusConflict, "the owner is already set up; sign in instead")
		return
	}
	if !a.hostAllowed(r) {
		fail(w, http.StatusForbidden, "unknown host name; add it to "+EnvAllowedHosts)
		return
	}
	a.mu.Lock()
	want := a.setupCode
	a.mu.Unlock()
	if want == "" || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(c.Code)), []byte(want)) != 1 {
		log.Printf("auth: first-run setup with a wrong setup code from %s", r.RemoteAddr)
		fail(w, http.StatusForbidden, "wrong setup code; find it in the server log or in "+SetupCodeFile+" in the config folder")
		return
	}
	h, err := hashPassword(c.Password, a.hash)
	if err != nil {
		log.Printf("auth: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	var id int64
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM users WHERE role = 'owner'").Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return errOwnerExists
		}
		var err error
		id, err = store.Insert(tx, "users", &domain.User{Name: c.Name, Role: "owner", PasswordHash: h, State: "active", Perms: "[]"}, domain.System)
		return err
	})
	if errors.Is(err, errOwnerExists) {
		fail(w, http.StatusConflict, "the owner is already set up; sign in instead")
		return
	}
	if err != nil {
		log.Printf("auth: setup: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.mu.Lock()
	a.setupCode = ""
	a.mu.Unlock()
	if err := os.Remove(filepath.Join(a.s.Dir(), SetupCodeFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("auth: could not remove the used setup code: %v", err)
	}
	log.Printf("auth: first-run setup made the owner %q from %s", c.Name, r.RemoteAddr)
	a.startSession(w, r, id, c.Name, http.StatusCreated)
}

var errOwnerExists = errors.New("owner exists")
var errNoLocalSignIn = errors.New("signs in only through an outside account")

// clientKey is the key every sign-in limit uses: the proxy-aware ClientKey,
// so behind a trusted proxy each visitor gets their own budget.
func (a *Auth) clientKey(r *http.Request) string { return a.ClientKey(r) }

// ClientKey names the caller for rate limits: the peer address, or the
// address a trusted proxy forwards (the last X-Forwarded-For entry, else
// X-Real-IP). Forwarding headers from any other peer are ignored.
func (a *Auth) ClientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if peer := net.ParseIP(host); peer != nil && a.trustedProxy(peer) {
		client := strings.TrimSpace(r.Header.Get("X-Real-IP"))
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			client = strings.TrimSpace(parts[len(parts)-1])
		}
		if ip := net.ParseIP(client); ip != nil {
			host = ip.String()
		}
	}
	return "ip:" + host
}

// waitFor returns how long the key must still wait, or 0.
func (a *Auth) waitFor(key string, free int) time.Duration {
	f := a.fails[key]
	if f == nil || f.n < free {
		return 0
	}
	d := time.Second << min(f.n-free, 20)
	if d > MaxBackoff {
		d = MaxBackoff
	}
	if left := f.last.Add(d).Sub(a.now()); left > 0 {
		return left
	}
	return 0
}

func (a *Auth) noteFail(keys ...string) {
	now := a.now()
	if len(a.fails) > 10000 {
		for k, f := range a.fails {
			if now.Sub(f.last) > MaxBackoff {
				delete(a.fails, k)
			}
		}
	}
	for _, k := range keys {
		f := a.fails[k]
		if f == nil {
			f = &failure{}
			a.fails[k] = f
		}
		f.n++
		f.last = now
	}
}

// failMark is how one key stood before noteFail, so undoFail can put back
// its time as well as its count.
type failMark struct {
	key  string
	had  bool
	last time.Time
}

// markFails reads the keys before noteFail; the caller holds a.mu.
func (a *Auth) markFails(keys ...string) []failMark {
	out := make([]failMark, 0, len(keys))
	for _, k := range keys {
		m := failMark{key: k}
		if f := a.fails[k]; f != nil {
			m.had, m.last = true, f.last
		}
		out = append(out, m)
	}
	return out
}

// undoFail takes back the failure noted up front when the check could not
// run at all (the media server did not answer), so an outage never locks
// people out or restarts their wait. Each key loses one count, as parallel
// attempts noted their own.
func (a *Auth) undoFail(marks []failMark) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range marks {
		f := a.fails[m.key]
		if f == nil {
			continue
		}
		if f.n--; f.n <= 0 {
			delete(a.fails, m.key)
		} else if m.had {
			f.last = m.last
		}
	}
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if len(c.Name) > 64 { // no account has a longer name; keeps the back-off map small
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	ipKey, userKey := a.clientKey(r), "name:"+strings.ToLower(c.Name)
	// Check and reserve in one locked step: the attempt counts as failed up
	// front and is cleared on success, so parallel attempts cannot slip past.
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
	select {
	case a.checks <- struct{}{}:
		defer func() { <-a.checks }()
	case <-r.Context().Done():
		return
	}
	var id int64
	var name, hash string
	err := a.s.Read().QueryRowContext(r.Context(), "SELECT id, name, password_hash FROM users WHERE name = ? AND state = 'active'", c.Name).
		Scan(&id, &name, &hash)
	known := err == nil // a typed name that is no user may be a password typed in the wrong box
	if err != nil || hash == "" {
		// Same work whether the name is unknown or has no password (people
		// who sign in only through Plex, Jellyfin or Emby).
		hash = a.dummy
		err = errors.Join(err, errNoLocalSignIn)
	}
	good := CheckPassword(hash, c.Password) && err == nil
	if !good {
		log.Printf("auth: failed sign-in from %s", r.RemoteAddr)
		who := "unknown name"
		if known {
			who = name
		}
		a.failedEvent(r, who)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	a.mu.Lock()
	delete(a.fails, ipKey)
	delete(a.fails, userKey)
	a.mu.Unlock()
	a.passed(w, r, id, name)
}

func (a *Auth) setCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.behindHTTPS(r), MaxAge: maxAge})
}

// startSession drops the session the request came with (so a planted id
// never survives a sign-in) and starts a fresh one.
func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, userID int64, name string, code int) {
	a.startSessionAs(w, r, userID, name, code, AuthMethodLocalSetup)
}

// startSessionAs starts a fresh session with its authentication method. If
// the request presents an existing session, its complete rotation family is
// deleted before the replacement is made.
func (a *Auth) startSessionAs(w http.ResponseWriter, r *http.Request, userID int64, name string, code int, method string) {
	now := a.now()
	tok, csrf := token(32), token(32)
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if ck, err := r.Cookie(CookieName); err == nil {
			if err := deletePresentedFamily(tx, ck.Value); err != nil {
				return err
			}
		}
		t := store.FormatTime(now)
		localAt := ""
		if method == AuthMethodLocalPassword || method == AuthMethodLocalSetup {
			localAt = t
		}
		_, err := tx.Exec(`INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family, auth_method, local_auth_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sum(tok), userID, csrf, t, t, t, store.FormatTime(now.Add(AbsoluteTimeout)),
			truncate(r.UserAgent(), 200), newFamily(), method, localAt)
		return err
	})
	if err != nil {
		log.Printf("auth: session: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.setCookie(w, r, tok, int(AbsoluteTimeout.Seconds()))
	a.event(r, domain.Actor{Kind: "user", ID: userID}, "signed_in", name+" signed in", map[string]any{"name": name, "method": method})
	writeJSON(w, code, map[string]string{"name": name, "csrf": csrf})
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(store.TimeLayout, s)
	return t
}

// session checks the cookie, ends idle or old sessions, and rotates the id
// once an hour while the session is used.
func (a *Auth) session(w http.ResponseWriter, r *http.Request) (Who, bool) {
	ck, err := r.Cookie(CookieName)
	if err != nil || ck.Value == "" {
		return Who{}, false
	}
	h := sum(ck.Value)
	var who Who
	var created, rotated, seen, expires, perms, localAuthAt string
	err = a.s.Read().QueryRowContext(r.Context(), `SELECT `+userCols+`, s.csrf, s.created_at, s.rotated_at, s.last_seen_at, s.expires_at, s.auth_method, s.local_auth_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.hash = ? AND u.state = 'active'`, h).
		Scan(&who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry, &who.CSRF, &created, &rotated, &seen, &expires, &who.AuthMethod, &localAuthAt)
	if err != nil {
		return Who{}, false
	}
	a.fill(&who, perms)
	now := a.now()
	localAt := parseTime(localAuthAt)
	localMethod := who.AuthMethod == AuthMethodLocalPassword || who.AuthMethod == AuthMethodLocalSetup
	who.LocalRecent = localMethod && !localAt.IsZero() && !localAt.After(now) && now.Sub(localAt) < localRecentFor
	switch who.AuthMethod {
	case ProviderPlex, ProviderJellyfin, ProviderEmby:
		at := parseTime(created)
		who.OutsideRecent = !at.IsZero() && !at.After(now) && now.Sub(at) < localRecentFor
	}
	if !now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) > IdleTimeout {
		_ = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			_, err := tx.Exec("DELETE FROM sessions WHERE hash = ?", h)
			return err
		})
		a.setCookie(w, r, "", -1)
		return Who{}, false
	}
	who.Via = "session"
	t := store.FormatTime(now)
	switch {
	case now.Sub(parseTime(rotated)) > RotateEvery:
		tok := token(32)
		abs := parseTime(created).Add(AbsoluteTimeout)
		err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family, auth_method, local_auth_at)
				SELECT ?, user_id, csrf, created_at, ?, ?, expires_at, user_agent, family, auth_method, local_auth_at FROM sessions WHERE hash = ?`, sum(tok), t, t, h); err != nil {
				return err
			}
			// The old id lives on briefly for requests already in flight.
			grace := now.Add(rotateGrace)
			if grace.After(abs) {
				grace = abs
			}
			_, err := tx.Exec("UPDATE sessions SET expires_at = ?, rotated_at = ?, replaced = 1 WHERE hash = ?", store.FormatTime(grace), t, h)
			return err
		})
		if err == nil {
			a.setCookie(w, r, tok, int(abs.Sub(now).Seconds()))
		}
	case now.Sub(parseTime(seen)) > touchEvery:
		_ = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE sessions SET last_seen_at = ? WHERE hash = ?", t, h)
			return err
		})
	}
	return who, true
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if who, ok := From(r.Context()); ok && who.UserID != 0 {
		a.event(r, Actor(r.Context()), "signed_out", who.Name+" signed out", map[string]any{"name": who.Name})
	}
	if ck, err := r.Cookie(CookieName); err == nil {
		_ = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			return deletePresentedFamily(tx, ck.Value)
		})
	}
	a.setCookie(w, r, "", -1)
	// The browser drops what it kept for this person, such as Explore rows.
	w.Header().Set("Clear-Site-Data", `"cache"`)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) me(w http.ResponseWriter, r *http.Request) {
	who, _ := From(r.Context())
	// People who joined through a media server have no password until they set one.
	hasPassword := true
	_ = a.s.Read().QueryRowContext(r.Context(), "SELECT password_hash != '' FROM users WHERE id = ?", who.UserID).Scan(&hasPassword)
	writeJSON(w, http.StatusOK, map[string]any{"id": who.UserID, "name": who.Name, "via": who.Via, "csrf": who.CSRF, "role": who.Role,
		"perms": who.Effective(), "two_factor": who.TwoFactor, "must_two_factor": who.MustTwoFactor, "auth_method": who.AuthMethod,
		"local_recent": who.LocalRecent, "outside_recent": who.OutsideRecent, "has_password": hasPassword,
		"avatar": a.avatarOf(r.Context(), who.UserID), "safe_mode": a.safeMode, "max_rating": who.MaxRating, "rating_country": countryOr(who.RatingCountry)})
}

// Cleanup drops ended sessions.
func (a *Auth) Cleanup(ctx context.Context) error {
	now := a.now()
	expiresAt := store.FormatTime(now)
	idleBefore := store.FormatTime(now.Add(-IdleTimeout))
	var needed bool
	if err := a.s.Read().QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM sessions WHERE expires_at <= ? OR last_seen_at < ?
	)`, expiresAt, idleBefore).Scan(&needed); err != nil {
		return err
	}
	if !needed {
		return nil
	}
	return a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM sessions WHERE expires_at <= ? OR last_seen_at < ?",
			expiresAt, idleBefore)
		return err
	})
}

func (a *Auth) checkKey(ctx context.Context, k string) (Who, bool) {
	if !strings.HasPrefix(k, keyPrefix) {
		return Who{}, false
	}
	var who Who
	var keyID int64
	var used sql.NullString
	var perms, scope string
	err := a.s.Read().QueryRowContext(ctx, `SELECT k.id, k.label, k.last_used_at, k.scope, `+userCols+` FROM api_keys k JOIN users u ON u.id = k.owner_user_id
		WHERE k.hash = ? AND u.state = 'active'`, sum(k)).Scan(&keyID, &who.KeyLabel, &used, &scope, &who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry)
	if err != nil {
		return Who{}, false
	}
	a.fill(&who, perms)
	who.Scopes = parseScopes(scope)
	now := a.now()
	if !used.Valid || now.Sub(parseTime(used.String)) > touchEvery {
		_ = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", store.FormatTime(now), keyID)
			return err
		})
	}
	who.Via = "api_key"
	who.KeyID = keyID
	return who, true
}

func (a *Auth) makeKey(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	var body struct {
		Label  string   `json:"label"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"label": "...", "scopes": ["all"]}`)
		return
	}
	body.Label = strings.TrimSpace(body.Label)
	if body.Label == "" || len(body.Label) > 64 {
		fail(w, http.StatusBadRequest, "give the key a label of 1 to 64 characters")
		return
	}
	who, _ := From(r.Context())
	if len(body.Scopes) == 0 {
		body.Scopes = []string{ScopeAll}
	}
	for _, sc := range body.Scopes {
		if !slices.Contains(Scopes, sc) {
			fail(w, http.StatusBadRequest, "scopes are "+strings.Join(Scopes, ", ")+" (all includes read, write and admin)")
			return
		}
		if !who.HasScope(sc) {
			fail(w, http.StatusForbidden, "a key cannot make a key with a scope it does not hold")
			return
		}
		if sc == ScopeControl && !who.Has(needUsers) {
			fail(w, http.StatusForbidden, "only the owner and admins may make a key with the control scope")
			return
		}
	}
	if slices.Contains(body.Scopes, ScopeMCP) && len(slices.Compact(slices.Sorted(slices.Values(body.Scopes)))) > 1 {
		fail(w, http.StatusBadRequest, "an AI key (mcp) holds no other scope")
		return
	}
	body.Scopes = slices.Compact(slices.Sorted(slices.Values(body.Scopes)))
	scopeJSON, _ := json.Marshal(body.Scopes)
	k := keyPrefix + token(32)
	now := store.FormatTime(a.now())
	actor, _ := json.Marshal(Actor(r.Context()))
	var id int64
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO api_keys(created_at, created_by, updated_at, updated_by, owner_user_id, label, scope, hash, hint)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, now, string(actor), now, string(actor), who.UserID, body.Label, string(scopeJSON), sum(k), k[len(k)-4:])
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		log.Printf("auth: key: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.event(r, Actor(r.Context()), "api_key_created", "API key "+body.Label+" created", map[string]any{"key_id": id, "label": body.Label, "scopes": body.Scopes})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "label": body.Label, "scopes": body.Scopes, "key": k,
		"note": "This key is shown only now. Send it in the " + APIKeyHeader + " header."})
}

// KeyView is an API key without its value.
type KeyView struct {
	ID         int64    `json:"id"`
	Label      string   `json:"label"`
	Hint       string   `json:"hint"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	Scopes     []string `json:"scopes"`
}

// parseScopes reads a key's stored scope list, keeping known scopes.
func parseScopes(s string) []string {
	var raw []string
	_ = json.Unmarshal([]byte(s), &raw)
	out := []string{}
	for _, sc := range raw {
		if slices.Contains(Scopes, sc) && !slices.Contains(out, sc) {
			out = append(out, sc)
		}
	}
	return out
}

func (a *Auth) listKeys(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	// The owner sees every key; others see their own.
	who, _ := From(r.Context())
	rows, err := a.s.Read().QueryContext(r.Context(), "SELECT id, label, hint, created_at, last_used_at, scope FROM api_keys WHERE ? OR owner_user_id = ? ORDER BY id",
		who.Role == RoleOwner, who.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	defer rows.Close()
	out := []KeyView{}
	for rows.Next() {
		var k KeyView
		var used sql.NullString
		var scope string
		if err := rows.Scan(&k.ID, &k.Label, &k.Hint, &k.CreatedAt, &used, &scope); err != nil {
			fail(w, http.StatusInternalServerError, "something went wrong; see the log")
			return
		}
		k.Hint = "…" + k.Hint
		k.Scopes = parseScopes(scope)
		if used.Valid {
			k.LastUsedAt = &used.String
		}
		out = append(out, k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (a *Auth) deleteKey(w http.ResponseWriter, r *http.Request) {
	if !RequireStrong(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	var n int64
	var label string
	who, _ := From(r.Context())
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		_ = tx.QueryRow("SELECT label FROM api_keys WHERE id = ?", id).Scan(&label)
		res, err := tx.Exec("DELETE FROM api_keys WHERE id = ? AND (? OR owner_user_id = ?)", id, who.Role == RoleOwner, who.UserID)
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	if n == 0 {
		fail(w, http.StatusNotFound, "no such key")
		return
	}
	a.event(r, Actor(r.Context()), "api_key_deleted", "API key "+label+" deleted", map[string]any{"key_id": id, "label": label})
	w.WriteHeader(http.StatusNoContent)
}

// event writes a sign-in or API key Event with the caller's address. It
// never holds a password or a key.
func (a *Auth) event(r *http.Request, by domain.Actor, kind, summary string, data map[string]any) {
	data["ip"] = strings.TrimPrefix(a.ClientKey(r), "ip:")
	_ = a.s.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: by, Kind: kind, Summary: summary, Data: data})
}

// failedEvent logs a failed sign-in. Repeats from the same address for the
// same name within 10 minutes of the first add to that row's count and
// last_at instead of making new rows.
func (a *Auth) failedEvent(r *http.Request, name string) {
	ip := strings.TrimPrefix(a.ClientKey(r), "ip:")
	now := a.now().UTC()
	ctx := context.WithoutCancel(r.Context())
	var n int64
	_ = a.s.WriteEvents(ctx, func(tx *sql.Tx) error {
		// One row per address and name; at stays the first attempt, so the
		// 10 minutes count from it.
		res, err := tx.Exec(`UPDATE events SET data = json_set(data, '$.count', coalesce(json_extract(data, '$.count'), 1) + 1, '$.last_at', ?),
			updated_at = ? WHERE id = (SELECT id FROM events WHERE kind = 'sign_in_failed' AND json_extract(data, '$.ip') = ? AND json_extract(data, '$.name') = ? AND at >= ? ORDER BY id DESC LIMIT 1)`,
			store.FormatTime(now), store.FormatTime(now), ip, name, store.FormatTime(now.Add(-10*time.Minute)))
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if n > 0 {
		return
	}
	_ = a.s.AddEvent(ctx, &domain.Event{At: now, Kind: "sign_in_failed", Summary: "Sign-in failed for " + name,
		Data: map[string]any{"name": name, "ip": ip, "count": 1, "last_at": store.FormatTime(now)}})
}
