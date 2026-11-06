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
	// knownPlaceFor: an address that signed in as a name skips that name's
	// wait for this long (see signInWait).
	knownPlaceFor = 30 * 24 * time.Hour
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
	// ConfigUnmapped tells the setup page that /config is not mapped to the
	// host, so it can show a note before the owner is made.
	ConfigUnmapped bool
	fastHash       bool
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
	localNets       atomic.Pointer[localNets]
	dummy           string
	hosts           map[string]bool          // localhost and names from Options and the environment; fixed
	savedHosts      atomic.Pointer[[]string] // names saved in Settings (hosts.go)
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
	configUnmapped  bool
	sso             atomic.Pointer[ssoState] // swapped whole when single sign-on settings change; read with ssoNow
	ssoOpt          SSOOptions               // as started: who owns single sign-on, and the address rules
	// ssoMu fences sign-ins against changes: a single sign-on or trusted proxy
	// finish holds it to read until its session or two-factor step exists; a
	// settings change or an unlink holds it to write, so it ends what those made.
	ssoMu sync.RWMutex

	mu           sync.Mutex
	fails        map[string]*failure
	pruneAt      int // fails size at which pruneFails runs again
	lastPruneAt  time.Time
	realNames    map[string]bool // lower-case user names, for the full-table rule (isRealName)
	realNamesAt  time.Time
	satLogged    time.Time
	proxyWarned  atomic.Int64                    // unix nanoseconds of the last untrusted-proxy warning (localnet.go)
	known        map[string]map[string]time.Time // name -> address -> last full sign-in (signInWait)
	setupFails   map[string]*failure             // wrong setup codes (hosts.go)
	refusedHosts map[string]bool                 // refused names already logged (hosts.go)
	setupCode    string
	// codes are AI app sign-ins waiting for their key (aioauth.go).
	codes aiCodes
	// cimd reads AI apps' client ID metadata documents (aicimd.go).
	cimd aiCIMD
	// The first-run search for Jellyfin and Emby (setup_outside.go): where
	// it is sent and how long it waits (defaults when empty), one at a time.
	discoverTo   string
	discoverWait time.Duration
	discovering  atomic.Bool
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
	a.vault, a.http, a.plexURL, a.plexApp, a.safeMode, a.configUnmapped = opt.Vault, opt.HTTP, opt.PlexURL, opt.PlexAppURL, opt.SafeMode, opt.ConfigUnmapped
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
		if h = normHost(h); h != "" {
			a.hosts[h] = true
		}
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
	// No setup code (decisions/0009): a code file left by an older version goes.
	if err := os.Remove(filepath.Join(a.s.Dir(), SetupCodeFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("auth: could not remove an old setup code file: %v", err)
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
	a.localNets.Store(a.loadLocalNets(context.Background()))
	a.loadKnown(context.Background())
	a.loadSavedHosts(context.Background())
	a.requireAdmin2FA.Store(a.signInSetting(context.Background(), keyRequireAdmin2FA) == "true")
	a.initSSO(opt.SSO)
	a.ssoOpt = opt.SSO
	if !opt.SSO.OIDCFromEnv {
		a.loadSavedOIDC(context.Background())
	}
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
	// SetupRecent is true for the session first-run owner setup started,
	// until the first-run wizard is done (at most setupRecentFor); it then
	// counts as LocalRecent.
	SetupRecent bool `json:"setup_recent"`
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
		"POST /api/v1/auth/media-server/jellyfin", "POST /api/v1/auth/media-server/emby",
		"POST /api/v1/auth/setup/plex/start", "POST /api/v1/auth/setup/plex/finish",
		"POST /api/v1/auth/setup/media-server/jellyfin", "POST /api/v1/auth/setup/media-server/emby",
		"GET /api/v1/auth/setup/discover":
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
	// Joining through an invite with Plex, Jellyfin or Emby.
	if r.Method == http.MethodPost && inviteOutsidePath(r.URL.Path) {
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

func (a *Auth) trustedProxy(ip net.IP) bool {
	for _, n := range a.proxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// localPeer says whether the request comes from a listed local network,
// reading the caller's address by the shared rule in clientAddr.
func (a *Auth) localPeer(r *http.Request) bool {
	addr, ok := a.clientAddr(r)
	return ok && a.isLocal(addr)
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
		// The old-app API takes only its own keys. Like an API key, such a
		// key is no browser credential, so the host and origin checks do
		// not apply (Seerr calls from a Docker neighbour by name). A
		// missing or wrong key gets a bare 401, as the old apps answer.
		if to, ok := compatLink(r); ok {
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if side := CompatSide(r.URL.Path); side != "" {
			k, _ := compatKey(r)
			who, ok := a.checkCompatKey(r.Context(), side, k)
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, who)))
			return
		}
		if aiPublic(r) {
			// AI app sign-in: no cookies are read or set, so no origin check.
			// The host name is still checked, since the answers echo it.
			if !a.hostAllowed(r) {
				a.logRefusedHost(r.Host)
				fail(w, http.StatusForbidden, hostRefusedMsg(r.Host))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if public(r) {
			// First-run setup checks the host name itself: a home name is
			// accepted there with no proxy in between (hosts.go).
			setup := r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/setup" || setupOutsidePath(r)
			if unsafeMethod(r.Method) && a.refuse(w, r, !setup) {
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
		if k == "" && mcpPath(r.URL.Path) {
			// An AI app without a key is told where to sign in, even on the
			// local network or beside a browser session.
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.aiResourceMetadataURL(r)+`", scope="mcp"`)
			fail(w, http.StatusUnauthorized, "sign in: add HolyArrs in your AI app and allow it in the browser")
			return
		}
		if k != "" {
			who, ok := a.checkKey(r.Context(), k)
			if !ok {
				if mcpPath(r.URL.Path) {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", resource_metadata="`+a.aiResourceMetadataURL(r)+`"`)
				}
				fail(w, http.StatusUnauthorized, "API key not accepted")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, who)))
			return
		}
		if who, ok := a.session(w, r); ok {
			if unsafeMethod(r.Method) {
				if a.refuse(w, r, true) {
					return
				}
				if subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(who.CSRF)) != 1 {
					fail(w, http.StatusForbidden, "missing or wrong CSRF token")
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, who)))
			return
		}
		if a.skipLocal.Load() && a.hostAllowed(r) && a.localPeer(r) {
			if id, name, ok := a.owner(r.Context()); ok {
				if unsafeMethod(r.Method) && a.refuse(w, r, true) {
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
	m.HandleFunc("POST /api/v1/auth/setup/done", a.setupDone)
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
	a.plexMineRoutes(m)
	a.ssoRoutes(m)
	a.aiOAuthRoutes(m)
}

func (a *Auth) state(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.owner(r.Context())
	out := map[string]any{"needs_setup": !ok, "ways": a.ways(r.Context()), "safe_mode": a.safeMode,
		"min_password_length": MinPasswordLength(r.Context(), a.s.Read())}
	if a.configUnmapped {
		out["config_unmapped"] = true
	}
	if !ok {
		out["setup_ways"] = setupWays
	}
	writeJSON(w, http.StatusOK, out)
}

type creds struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"` // ignored: first run has no setup code (decisions/0009)
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
	if msg := nameProblem(c.Name); msg != "" {
		fail(w, http.StatusBadRequest, msg)
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
	// INTENTIONAL (Luis, 2026-11-05, decisions/0009-no-setup-code.md): no
	// setup code. Whoever opens a fresh HolyArrs first makes the owner, as in
	// Overseerr; `holyarrs reset-owner` takes it back. Do not re-add a code,
	// a claim window or a first-run token, and do not suggest it in reviews.
	//
	// A home name (tower.local, nas) is accepted here only with no proxy in
	// between; it is then saved.
	newHost := ""
	if !a.hostAllowed(r) {
		h := normHost(r.Host)
		if !homeName(h) || len(a.proxies) > 0 || forwarded(r) {
			a.logRefusedHost(r.Host)
			fail(w, http.StatusForbidden, hostRefusedMsg(r.Host))
			return
		}
		newHost = h
	}
	h, err := hashPassword(c.Password, a.hash)
	if err != nil {
		log.Printf("auth: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	var id int64
	var saved []string
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
		if err != nil || newHost == "" {
			return err
		}
		saved = append(slices.Clone(a.savedHostList()), newHost)
		v, _ := json.Marshal(saved)
		return putSetting(tx, settingsGroup, signInPrefix+keyAllowedHosts, string(v), domain.System, a.now())
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
	if newHost != "" {
		a.savedHosts.Store(&saved)
		log.Printf("auth: first-run setup on %s; the name is saved under Allowed host names", newHost)
	}
	log.Printf("auth: first-run setup made the owner %q from %s", c.Name, r.RemoteAddr)
	a.startSetupSession(w, r, id, c.Name, http.StatusCreated, AuthMethodLocalSetup)
}

var errOwnerExists = errors.New("owner exists")
var errNoLocalSignIn = errors.New("signs in only through an outside account")

// clientKey is the key every sign-in limit uses: the proxy-aware ClientKey,
// so behind a trusted proxy each visitor gets their own budget.
func (a *Auth) clientKey(r *http.Request) string { return a.ClientKey(r) }

// ClientKey names the caller for rate limits: the peer address, or the
// client a trusted proxy forwards, by the shared rule in clientAddr.
// Forwarding headers from any other peer are ignored.
func (a *Auth) ClientKey(r *http.Request) string {
	k, _ := a.clientKeySure(r)
	return k
}

// clientKeySure is ClientKey plus whether the address is sure enough to stand
// for a known place (false when the proxy headers disagree).
func (a *Auth) clientKeySure(r *http.Request) (string, bool) {
	addr, _, unsure := a.clientAddrUnsure(r)
	return addrKey(addr, unsure), !unsure
}

// addrKey is the limit key of a client address. When the proxy headers
// disagree, the X-Real-IP address goes under its own "xri:" namespace: the
// visitor may have written it, so it must never read or charge the "ip:"
// count of a real visitor at that address.
func addrKey(addr string, unsure bool) string {
	if unsure {
		return "xri:" + limitAddr(addr)
	}
	return "ip:" + limitAddr(addr)
}

// clientKeys is clientKeySure plus altKey: when the proxy headers disagree,
// the key of the other header's address ("" otherwise). A failure counts under
// both and the larger wait stands. altKey has its own "xff:" namespace, read
// and charged only by requests whose headers disagree: a visitor can write any
// address in the header their proxy does not set, so that address's "ip:"
// count (a real visitor's) must never be touched by it.
func (a *Auth) clientKeys(r *http.Request) (ipKey, altKey string, sure bool) {
	addr, other, _, unsure := a.clientAddrBoth(r)
	if other != "" {
		altKey = "xff:" + limitAddr(other)
	}
	return addrKey(addr, unsure), altKey, !unsure
}

// altWait is the address wait for altKey, if there is one; the caller holds a.mu.
func (a *Auth) altWait(altKey string) time.Duration {
	if altKey == "" {
		return 0
	}
	return a.waitFor(altKey, freeTries)
}

// place is where a sign-in came from, for known places.
type place struct {
	ipKey, userKey string
	sure           bool
}

// limitAddr is the address as limits count it: IPv4 as is (an IPv4-mapped
// IPv6 address as its IPv4 form), IPv6 as its /64, since one network owns a
// whole /64 and could otherwise bring unlimited fresh addresses. The
// local-network skip reads the exact address, not this.
func limitAddr(addr string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return addr
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// eventAddr is the exact caller address, for the Events log.
func (a *Auth) eventAddr(r *http.Request) string {
	addr, _ := a.clientAddr(r)
	if ip := net.ParseIP(addr); ip != nil {
		return ip.String()
	}
	return addr
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

// signInWait is how long a password sign-in must wait; the caller holds a.mu.
// The per-address wait always counts. The per-name wait counts for every
// address except one that signed in as that name in the last 30 days, so
// strangers spreading guesses over many addresses stay slowed, yet cannot
// keep the owner out of a place they used before (home, their phone's line).
func (a *Auth) signInWait(ipKey, userKey string, sure bool) time.Duration {
	wait := a.waitFor(ipKey, freeTries)
	if a.knownPlace(ipKey, userKey, sure) {
		return wait
	}
	return max(wait, a.waitFor(userKey, freeUserTries))
}

// knownPlace says whether ipKey signed in as userKey in the last 30 days.
func (a *Auth) knownPlace(ipKey, userKey string, sure bool) bool {
	if !sure {
		return false
	}
	t, ok := a.known[userKey][ipKey]
	return ok && a.now().Sub(t) < knownPlaceFor
}

const (
	knownPerName = 20   // places kept for one name
	knownNames   = 2000 // names kept at all
)

// noteKnown remembers a finished sign-in for signInWait; the caller holds
// a.mu and passes the result to saveKnown once a.mu is unlocked, so the place
// still counts after a restart. Places are kept per name, so other names and
// addresses can never push a name's places out; only that name's own least
// recent place goes. An address that is not sure is never recorded.
func (a *Auth) noteKnown(p place) *knownChange {
	if !p.sure || p.userKey == "" {
		return nil
	}
	ch := &knownChange{put: p, at: a.now()}
	ch.gone = a.rememberKnown(p.userKey, p.ipKey, ch.at)
	return ch
}

// knownChange is one noteKnown for saveKnown: the place to store, and the
// places it pushed out (an empty ip key: every place of that name).
type knownChange struct {
	put  place
	at   time.Time
	gone [][2]string
}

// rememberKnown records userKey signing in from ipKey at the given time and
// returns the places pushed out to stay within the limits; the caller holds
// a.mu.
func (a *Auth) rememberKnown(userKey, ipKey string, at time.Time) (gone [][2]string) {
	if a.known == nil {
		a.known = map[string]map[string]time.Time{}
	}
	set := a.known[userKey]
	if set == nil {
		if len(a.known) >= knownNames {
			var oldest string
			var oldestAt time.Time
			for n, s := range a.known {
				var last time.Time
				for _, t := range s {
					if t.After(last) {
						last = t
					}
				}
				if oldest == "" || last.Before(oldestAt) {
					oldest, oldestAt = n, last
				}
			}
			delete(a.known, oldest)
			gone = append(gone, [2]string{oldest, ""})
		}
		set = map[string]time.Time{}
		a.known[userKey] = set
	}
	if _, ok := set[ipKey]; !ok && len(set) >= knownPerName {
		var oldest string
		var oldestAt time.Time
		for k, t := range set {
			if oldest == "" || t.Before(oldestAt) {
				oldest, oldestAt = k, t
			}
		}
		delete(set, oldest)
		gone = append(gone, [2]string{userKey, oldest})
	}
	set[ipKey] = at
	return gone
}

// saveKnown stores a noteKnown change, and drops that name's places older
// than knownPlaceFor. Best effort: a failed write is logged and never fails
// the sign-in; the place still counts until the next restart.
func (a *Auth) saveKnown(ch *knownChange) {
	if ch == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		for _, g := range ch.gone {
			q, args := "DELETE FROM known_places WHERE user_key = ? AND ip_key = ?", []any{g[0], g[1]}
			if g[1] == "" {
				q, args = "DELETE FROM known_places WHERE user_key = ?", []any{g[0]}
			}
			if _, err := tx.Exec(q, args...); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("DELETE FROM known_places WHERE user_key = ? AND last_at < ?",
			ch.put.userKey, store.FormatTime(ch.at.Add(-knownPlaceFor))); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO known_places(user_key, ip_key, last_at) VALUES (?, ?, ?)
			ON CONFLICT(user_key, ip_key) DO UPDATE SET last_at = excluded.last_at`, ch.put.userKey, ch.put.ipKey, store.FormatTime(ch.at))
		return err
	})
	if err != nil {
		log.Printf("auth: could not save a known sign-in place: %v", err)
	}
}

// loadKnown reads the known places of the last 30 days at start, oldest
// first, under the same limits as noteKnown.
func (a *Auth) loadKnown(ctx context.Context) {
	rows, err := a.s.Read().QueryContext(ctx, "SELECT user_key, ip_key, last_at FROM known_places WHERE last_at >= ? ORDER BY last_at",
		store.FormatTime(a.now().Add(-knownPlaceFor)))
	if err != nil {
		log.Printf("auth: could not read known sign-in places: %v", err)
		return
	}
	defer rows.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	for rows.Next() {
		var u, ip, at string
		if rows.Scan(&u, &ip, &at) == nil {
			if t := parseTime(at); !t.IsZero() {
				a.rememberKnown(u, ip, t)
			}
		}
	}
}

func (a *Auth) noteFail(keys ...string) {
	now := a.now()
	if len(a.fails) >= maxTracked && (len(a.fails) >= a.pruneAt || now.Sub(a.lastPruneAt) > MaxBackoff) {
		a.pruneFails(now)
		// Scan again only after the map has grown a little, so a flood that
		// leaves nothing to free does not cost a full scan per attempt.
		a.pruneAt = len(a.fails) + maxTracked/10
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		f := a.fails[k]
		if f == nil {
			// A name record is only added while the map has room for it, so
			// a flood of made-up names never pushes out a record that still
			// holds a wait. The address key still counts. Names use at most
			// three quarters of the room, leaving the rest for addresses.
			limit := 4 * maxTracked
			if protectedKey(k) {
				limit = 3 * maxTracked
			}
			if len(a.fails) >= limit && protectedKey(k) && now.Sub(a.lastPruneAt) >= time.Minute {
				a.pruneFails(now) // free expired records before refusing
			}
			if len(a.fails) >= limit {
				// A table full of names: made-up names get no record (no account
				// to protect); a real account still gets one, so a flood cannot
				// strip its wait. Real names are few (one per user), so this
				// stays bounded. Trade-off: only while a flood has filled the
				// table can an attacker tell real names from made-up ones, by
				// whether a name wait appears; fine for a self-hosted app with a
				// few users.
				// The ceiling sits above the address share, so address records
				// from the flood cannot use up the room real names get.
				if !protectedKey(k) || !a.isRealName(k, now) || len(a.fails) >= 4*maxTracked+fullExtra {
					if protectedKey(k) {
						a.noteSkipped(now)
					}
					continue
				}
			}
			f = &failure{}
			a.fails[k] = f
		}
		// An address count whose wait ended long ago starts over, so one
		// typo now and then at home never builds up to a lockout (pruning
		// does the same, but only once the table is big). A real account
		// keeps its count for accountKeep.
		if idle := now.Sub(f.last); f.n > 0 && idle > MaxBackoff && (idle > accountKeep || !a.accountKey(k, now)) {
			f.n = 0
		}
		f.n++
		f.last = now
	}
}

// noteSkipped logs once per window that made-up names get no record.
func (a *Auth) noteSkipped(now time.Time) {
	if now.Sub(a.satLogged) > MaxBackoff {
		a.satLogged = now
		log.Printf("auth: sign-in limits are full; names with no account are no longer tracked until the flood stops")
	}
}

// isRealName says whether a name:/2fa: key is for an account (a second-step
// key only exists for a ticket, so always). The user names are read once a
// minute, and only while the table is full; the caller holds a.mu.
func (a *Auth) isRealName(k string, now time.Time) bool {
	rest, ok := strings.CutPrefix(k, "name:")
	if !ok {
		return true
	}
	if a.realNames == nil || now.Sub(a.realNamesAt) >= time.Minute {
		names := map[string]bool{}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// A Jellyfin or Emby account linked to a member under another name
		// (taken, or renamed since) counts as real too, as "<kind>:<name>".
		if rows, err := a.s.Read().QueryContext(ctx, `SELECT name FROM users UNION
			SELECT provider || ':' || external_name FROM user_links WHERE provider IN ('jellyfin', 'emby') AND external_name != ''`); err == nil {
			for rows.Next() {
				var n string
				if rows.Scan(&n) == nil {
					names[strings.ToLower(n)] = true
				}
			}
			_ = rows.Close()
		}
		a.realNames, a.realNamesAt = names, now
	}
	if a.realNames[rest] {
		return true
	}
	// name:<kind>:<name> for a Jellyfin or Emby sign-in; only those kinds,
	// so a typed name like "x1:ana" is not taken for the account ana.
	kind, after, ok := strings.Cut(rest, ":")
	return ok && (kind == ProviderJellyfin || kind == ProviderEmby) && a.realNames[after]
}

// maxTracked bounds the failure and known-place maps.
const maxTracked = 10000

// fullExtra caps the real-account records noteFail adds once the table is
// full, a hard ceiling in case names that look real are made up.
const fullExtra = 1000

// protectedKey says whether a fail key is a per-name one (sign-in name or
// second-step ticket), which is kept for as long as its wait runs.
func protectedKey(k string) bool {
	return strings.HasPrefix(k, "name:") || strings.HasPrefix(k, "2fa:")
}

// accountKeep is how long a real account's count is kept after its last
// failure, so a quiet spell between guessing rounds does not hand back free
// guesses. Its wait still ends after MaxBackoff; only the level is kept.
const accountKeep = 24 * time.Hour

// accountPrefixes are fail keys that name a user id, so their number is
// bounded by the users.
var accountPrefixes = []string{"2fa:", "reauth:", "pw:", "code:", "sso-proof:", "sso-test:"}

// accountKey says whether a fail record belongs to a real account; the caller
// holds a.mu.
func (a *Auth) accountKey(k string, now time.Time) bool {
	for _, p := range accountPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return strings.HasPrefix(k, "name:") && a.isRealName(k, now)
}

// pruneFails frees room in a.fails; the caller holds a.mu. Records whose wait
// is over go first; a real account's record only after accountKeep. If the
// map is still full (a flood of fresh addresses), the oldest address records
// go. Per-name and account records are never evicted that way: there are few
// real names, and flushing them would end the wait that protects an account.
// When names fill their share, noteFail stops adding new ones instead, so
// memory stays bounded and a flood of made-up names cannot remove the
// owner's wait.
func (a *Auth) pruneFails(now time.Time) {
	a.lastPruneAt = now
	for k, f := range a.fails {
		if idle := now.Sub(f.last); idle > MaxBackoff && (idle > accountKeep || !a.accountKey(k, now)) {
			delete(a.fails, k)
		}
	}
	if len(a.fails) < maxTracked {
		return
	}
	type kf struct {
		k    string
		last time.Time
	}
	var addrs []kf
	for k, f := range a.fails {
		if !protectedKey(k) && !a.accountKey(k, now) {
			addrs = append(addrs, kf{k, f.last})
		}
	}
	slices.SortFunc(addrs, func(x, y kf) int { return x.last.Compare(y.last) })
	for _, e := range addrs[:min(len(addrs)/5+1, len(addrs))] {
		delete(a.fails, e.k)
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
	a.undoFailLocked(marks)
}

// undoFailLocked is undoFail for a caller that holds a.mu. A good sign-in
// uses it to take back only its own reservation on a shared address, so it
// never clears the count other people's failures left there.
func (a *Auth) undoFailLocked(marks []failMark) {
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
	ipKey, altKey, sure := a.clientKeys(r)
	userKey := "name:" + strings.ToLower(c.Name)
	// Check and reserve in one locked step: the attempt counts as failed up
	// front and is cleared on success, so parallel attempts cannot slip past.
	var marks []failMark
	a.mu.Lock()
	wait := max(a.signInWait(ipKey, userKey, sure), a.altWait(altKey))
	if wait == 0 {
		marks = a.markFails(ipKey, altKey)
		a.noteFail(ipKey, altKey, userKey)
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
	var name, hash, state string
	// One lookup in every case (no extra query on a failure path), so the
	// time taken does not tell an unknown name from a turned-off member.
	err := a.s.Read().QueryRowContext(r.Context(), "SELECT id, name, password_hash, state FROM users WHERE name = ?", c.Name).
		Scan(&id, &name, &hash, &state)
	known := err == nil // a typed name that is no user may be a password typed in the wrong box
	turnedOff := known && state != "active"
	if turnedOff {
		err = errDisabled
	}
	if err != nil || hash == "" {
		// Same work whether the name is unknown or has no password (people
		// who sign in only through Plex, Jellyfin or Emby).
		hash = a.dummy
		err = errors.Join(err, errNoLocalSignIn)
	}
	good := CheckPassword(hash, c.Password) && err == nil
	if !good {
		log.Printf("auth: failed sign-in from %s", r.RemoteAddr)
		// A turned-off member keeps their name; a name that is no member stays
		// "unknown name" (it may be a password typed in the wrong box).
		who, how := unknownName, "password"
		if known {
			who = name
		}
		if turnedOff {
			how = "turned off"
		}
		a.failedEvent(r, who, how)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	a.mu.Lock()
	a.undoFailLocked(marks) // only this attempt's address count; others' failures there stay
	delete(a.fails, userKey)
	a.mu.Unlock()
	a.passedPlace(w, r, id, name, AuthMethodLocalPassword, place{ipKey, userKey, sure})
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
	a.startSessionIn(w, r, userID, name, code, method, newFamily())
}

// startSetupSession starts the session first-run owner setup hands out; its
// family marks it for SetupRecent (see setupRecentFor).
func (a *Auth) startSetupSession(w http.ResponseWriter, r *http.Request, userID int64, name string, code int, method string) {
	a.startSessionIn(w, r, userID, name, code, method, setupFamily+newFamily())
}

func (a *Auth) startSessionIn(w http.ResponseWriter, r *http.Request, userID int64, name string, code int, method, family string) {
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
			truncate(r.UserAgent(), 200), family, method, localAt)
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
	var created, rotated, seen, expires, perms, localAuthAt, family string
	err = a.s.Read().QueryRowContext(r.Context(), `SELECT `+userCols+`, s.csrf, s.created_at, s.rotated_at, s.last_seen_at, s.expires_at, s.auth_method, s.local_auth_at, s.family
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.hash = ? AND u.state = 'active'`, h).
		Scan(&who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry, &who.CSRF, &created, &rotated, &seen, &expires, &who.AuthMethod, &localAuthAt, &family)
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
	if strings.HasPrefix(family, setupFamily) {
		at := parseTime(created)
		who.SetupRecent = !at.IsZero() && !at.After(now) && now.Sub(at) < setupRecentFor
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
		"local_recent": who.LocalRecent, "outside_recent": who.OutsideRecent, "setup_recent": who.SetupRecent, "has_password": hasPassword,
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
		if sc == ScopeCompat {
			fail(w, http.StatusBadRequest, "old-app API keys are made with POST /api/v1/settings/old-app-api/keys/{side}, not here")
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
	data["ip"] = a.eventAddr(r)
	_ = a.s.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: by, Kind: kind, Summary: summary, Data: data})
}

// unknownName stands for a typed sign-in name that is no member.
const unknownName = "unknown name"

// knownName is the member (in any state) a typed sign-in name belongs to,
// or unknownName.
func (a *Auth) knownName(ctx context.Context, typed string) string {
	var n string
	if a.s.Read().QueryRowContext(ctx, "SELECT name FROM users WHERE name = ?", typed).Scan(&n) != nil {
		return unknownName
	}
	return n
}

// failedSummary is a failed sign-in row's line: how it failed and for whom.
func failedSummary(name, how string) string {
	switch how {
	case "", "password":
		return "Sign-in failed for " + name
	case "two-factor":
		return "Wrong two-factor code for " + name
	case "turned off":
		return "Sign-in by turned-off account " + name
	}
	return "Sign-in failed for " + name + " (" + how + ")"
}

// failedEvent logs a failed sign-in and how it failed (password,
// two-factor, turned off, jellyfin, emby, not allowed). Repeats from the
// same address for the same name and way within 10 minutes of the first add
// to that row's count and last_at instead of making new rows.
func (a *Auth) failedEvent(r *http.Request, name, how string) {
	ip := a.eventAddr(r)
	now := a.now().UTC()
	ctx := context.WithoutCancel(r.Context())
	var n int64
	_ = a.s.WriteEvents(ctx, func(tx *sql.Tx) error {
		// One row per address and name; at stays the first attempt, so the
		// 10 minutes count from it.
		res, err := tx.Exec(`UPDATE events SET data = json_set(data, '$.count', coalesce(json_extract(data, '$.count'), 1) + 1, '$.last_at', ?),
			updated_at = ? WHERE id = (SELECT id FROM events WHERE kind = 'sign_in_failed' AND json_extract(data, '$.ip') = ? AND json_extract(data, '$.name') = ?
			AND coalesce(json_extract(data, '$.how'), 'password') = ? AND at >= ? ORDER BY id DESC LIMIT 1)`,
			store.FormatTime(now), store.FormatTime(now), ip, name, how, store.FormatTime(now.Add(-10*time.Minute)))
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if n > 0 {
		return
	}
	_ = a.s.AddEvent(ctx, &domain.Event{At: now, Kind: "sign_in_failed", Summary: failedSummary(name, how),
		Data: map[string]any{"name": name, "how": how, "ip": ip, "count": 1, "last_at": store.FormatTime(now)}})
}

// ssoRefusal is how an OIDC or forward-auth sign-in was refused, for its row.
func ssoRefusal(kind string, err error) string {
	switch {
	case errors.Is(err, errDisabled):
		return "turned off"
	case errors.Is(err, errNotAllowed):
		return kind + " not allowed"
	}
	return kind + " refused"
}

// EventAccountChanged is a change to someone's account: role, permissions,
// turned off or on, password set or changed, two-factor reset or turned on
// or off, sessions ended, an invite made. Never a password, code or key.
const EventAccountChanged = "account_changed"

// accountEvent writes an account_changed Event about the member userID.
func (a *Auth) accountEvent(r *http.Request, userID int64, action, summary string, extra map[string]any) {
	data := map[string]any{"user_id": userID, "action": action}
	for k, v := range extra {
		data[k] = v
	}
	a.event(r, Actor(r.Context()), EventAccountChanged, summary, data)
}

// userName is a member's name ("member <id>" when it cannot be read).
func (a *Auth) userName(ctx context.Context, id int64) string {
	var n string
	if a.s.Read().QueryRowContext(ctx, "SELECT name FROM users WHERE id = ?", id).Scan(&n) != nil || n == "" {
		return "member " + itoa(id)
	}
	return n
}

// byName is who made a change, for an Event line.
func byName(r *http.Request) string {
	if who, ok := From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "someone"
}
