// Package accounts decides who is signed in: the first-run owner, password,
// two-factor, single sign-on and media server sign-in with back-off, session
// cookies, CSRF checks, API keys, invites, people and their roles, and the
// "skip sign-in on the local network" choice (off by default).
package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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

// SetupCodeFile names the file older builds kept a first-run code in, inside
// the config folder. Start-up removes it.
const SetupCodeFile = "setup-code.txt"

// maxChecks caps password checks running at once.
const maxChecks = 4

// userCols are the users columns fill reads, prefixed with u.
const userCols = "u.id, u.name, u.role, u.perms, u.totp_secret_id IS NOT NULL, u.max_rating, u.rating_country"

// Auth guards every route.
type Auth struct {
	s               *database.Store
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

	mu              sync.Mutex
	fails           map[string]*failure
	pruneAt         int // fails size at which pruneFails runs again
	lastPruneAt     time.Time
	realNames       map[string]bool // lower-case user names, for the full-table rule (isRealName)
	realNamesAt     time.Time
	satLogged       time.Time
	proxyWarned     atomic.Int64                    // unix nanoseconds of the last untrusted-proxy warning (localnet.go)
	proxyMu         sync.Mutex                      // guards proxyPeer and proxyPeerAt
	proxyPeer       string                          // the untrusted proxy the warning saw (noteProxyPeer)
	proxyPeerAt     time.Time                       // when proxyPeer last sent forwarding headers
	known           map[string]map[string]time.Time // name -> address -> last full sign-in (signInWait)
	ownerSetupFails map[string]*failure             // failed first-run owner attempts (hosts.go)
	refusedHosts    map[string]bool                 // refused names already logged (hosts.go)
	// codes are AI app sign-ins waiting for their key (ai_signin.go).
	codes aiCodes
	// cimd reads AI apps' client ID metadata documents (ai_client_docs.go).
	cimd aiCIMD
	// The first-run search for Jellyfin and Emby (setup_outside.go): where
	// it is sent and how long it waits (defaults when empty), one at a time.
	discoverTo   string
	discoverWait time.Duration
	discovering  atomic.Bool
}

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

type failure struct {
	n    int
	last time.Time
}

// New makes Auth and loads its settings.
func New(s *database.Store, opt Options) *Auth {
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
	// Remove a code file an older build may have left.
	if err := os.Remove(filepath.Join(a.s.Dir(), SetupCodeFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("auth: could not remove an old setup code file: %v", err)
	}
	if opt.fastHash {
		a.hash = testHash
	}
	a.dummy, _ = hashPassword("not a real password", a.hash)
	a.skipLocal.Store(database.SettingOr(context.Background(), s.Read(), settingsGroup, settingSkipLocal, "") == "true")
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

// Cleanup drops ended sessions.
func (a *Auth) Cleanup(ctx context.Context) error {
	now := a.now()
	expiresAt := database.FormatTime(now)
	idleBefore := database.FormatTime(now.Add(-IdleTimeout))
	needed, err := endedSessionsExist(ctx, a.s.Read(), expiresAt, idleBefore)
	if err != nil {
		return err
	}
	if !needed {
		return nil
	}
	return a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		return deleteEndedSessions(ctx, tx, expiresAt, idleBefore)
	})
}

// fill completes who from a row of userCols and the sign-in settings.
func (a *Auth) fill(who *webserver.Caller, perms string) {
	who.Perms = parsePerms(perms)
	who.MustTwoFactor = who.Role == webserver.RoleAdmin && !who.TwoFactor && a.requireAdmin2FA.Load()
	who.RatingCountry = countryOr(who.RatingCountry)
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

func writeJSON(w http.ResponseWriter, code int, v any) { webserver.WriteJSONNoStore(w, code, v) }

func fail(w http.ResponseWriter, code int, msg string) { webserver.FailNoStore(w, code, msg) }

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(database.TimeLayout, s)
	return t
}
