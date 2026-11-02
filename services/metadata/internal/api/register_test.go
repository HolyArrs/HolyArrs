package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// registeringStore keeps self-registered credentials apart from the
// operator ones in memoryStore.
type registeringStore struct {
	*memoryStore
	mu      sync.Mutex
	sources map[string]string
	full    string
}

func (i *registeringStore) RegisterToken(_ context.Context, token, source, network string, _ int, _, _ time.Duration) error {
	if network == i.full {
		return model.ErrRegistryFull
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.sources[token] = source
	return nil
}

func (i *registeringStore) Registered(_ context.Context, token string, _ time.Duration) (string, bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	src, ok := i.sources[token]
	return src, ok, nil
}

// statsFixture is a made-up stats credential for these tests.
var statsFixture = strings.Join([]string{"fixture", "stats", "credential", "long"}, "-")

func register(s *Server, ip string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/installations", nil)
	r.Header.Set("CF-Connecting-IP", ip)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func registerToken(t *testing.T, s *Server, ip string) string {
	t.Helper()
	w := register(s, ip)
	var b struct {
		SchemaVersion int    `json:"schema_version"`
		Token         string `json:"token"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &b) != nil || b.SchemaVersion != 1 || len(b.Token) < 40 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	return b.Token
}

func registering(o Options) (*Server, *registeringStore) {
	s, mem, _ := setup(o)
	st := &registeringStore{memoryStore: mem, sources: map[string]string{}}
	s.store = st
	return s, st
}

func TestRegistrationClosedByDefault(t *testing.T) {
	s, _ := registering(Options{TrustedProxies: testProxy()})
	if w := register(s, "203.0.113.9"); w.Code != 405 {
		t.Fatalf("closed registration answered %d", w.Code)
	}
}

func TestRegistrationIssuesDistinctLookupOnlyCredentials(t *testing.T) {
	s, st := registering(Options{Registration: true, Burst: 100, StatsToken: statsFixture, TrustedProxies: testProxy()})
	a, b := registerToken(t, s, "203.0.113.10"), registerToken(t, s, "203.0.113.10")
	if a == b || st.sources[a] != "203.0.113.10" {
		t.Fatal("credentials not distinct or source not kept")
	}
	if w := reqFrom(s, lookup, a, "203.0.113.10"); w.Code != 200 {
		t.Fatalf("lookup refused %d", w.Code)
	}
	for _, path := range []string{"/metrics", "/v1/stats"} {
		if w := reqFrom(s, path, a, "203.0.113.10"); w.Code != 401 {
			t.Fatalf("%s reachable with a registered credential: %d", path, w.Code)
		}
	}
	if w := reqFrom(s, "/metrics", statsFixture, "203.0.113.10"); w.Code != 200 {
		t.Fatalf("metrics with stats token %d", w.Code)
	}
	if w := reqFrom(s, "/metrics", "synthetic-test-token", "203.0.113.10"); w.Code != 401 {
		t.Fatalf("metrics with an installation credential %d", w.Code)
	}
}

func TestRegistrationLimitedPerGroupAndGlobally(t *testing.T) {
	s, _ := registering(Options{Registration: true, RegisterPerHour: 2, RegisterGlobalPerHour: 4, TrustedProxies: testProxy()})
	registerToken(t, s, "2001:db8:1:2::10")
	registerToken(t, s, "2001:db8:1:2::99") // same /64
	w := register(s, "2001:db8:1:2:ffff::1")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "registration_limit") {
		t.Fatalf("/64 group limit %d %s", w.Code, w.Body.String())
	}
	registerToken(t, s, "203.0.113.21")
	registerToken(t, s, "203.0.113.22")
	if w := register(s, "203.0.113.23"); w.Code != 429 {
		t.Fatalf("global limit %d", w.Code)
	}
	if len(s.registrations) != 6 { // v6 /64 /48 /32; two v4 /32 and one /24
		t.Fatalf("global refusal created an address entry: %d", len(s.registrations))
	}
}

func TestRegistrationPassesClientAndAdmissionGates(t *testing.T) {
	s, _ := registering(Options{Registration: true, RegisterPerHour: 100, ClientPerMinute: 1, ClientBurst: 1, TrustedProxies: testProxy()})
	registerToken(t, s, "203.0.113.30")
	if w := register(s, "203.0.113.30"); w.Code != 429 || !strings.Contains(w.Body.String(), "client_limit") {
		t.Fatalf("client gate %d %s", w.Code, w.Body.String())
	}
	s, _ = registering(Options{Registration: true, MaxConcurrent: 1, TrustedProxies: testProxy()})
	s.admission <- struct{}{}
	if w := register(s, "203.0.113.31"); w.Code != 503 {
		t.Fatalf("admission gate %d", w.Code)
	}
}

func TestRegistrationFullTableEvictsOldest(t *testing.T) {
	s, _ := registering(Options{Registration: true, TokenSlots: 2, TrustedProxies: testProxy()})
	registerToken(t, s, "203.0.113.40")
	s.registrations["203.0.113.40"].updated = time.Now().Add(-time.Minute)
	registerToken(t, s, "203.0.113.41")
	registerToken(t, s, "203.0.113.42")
	if len(s.registrations) != 2 || s.registrations["203.0.113.40"] != nil {
		t.Fatalf("table not evicted by age: %v", len(s.registrations))
	}
}

func TestRegisteredTokenLimitDoesNotBlockSiblingsButSourceTotalHolds(t *testing.T) {
	s, _ := registering(Options{Registration: true, RegisterPerHour: 100, Burst: 2, PerMinute: 1, TrustedProxies: testProxy()})
	a, b := registerToken(t, s, "203.0.113.50"), registerToken(t, s, "203.0.113.50")
	for range 2 {
		if w := reqFrom(s, lookup, a, "198.51.100.1"); w.Code != 200 {
			t.Fatalf("within token limit %d", w.Code)
		}
	}
	if w := reqFrom(s, lookup, a, "198.51.100.1"); w.Code != 429 {
		t.Fatalf("token limit not applied %d", w.Code)
	}
	if w := reqFrom(s, lookup, b, "198.51.100.1"); w.Code != 200 {
		t.Fatalf("one token's limit blocked its sibling %d", w.Code)
	}
	// The source total is sourceFactor tokens' worth (8 here).
	ok := 3
	for range 6 {
		tok := registerToken(t, s, "203.0.113.50")
		for range 2 {
			if reqFrom(s, lookup, tok, "198.51.100.1").Code == 200 {
				ok++
			}
		}
	}
	if ok != 2*sourceFactor {
		t.Fatalf("source total allowed %d, want %d", ok, 2*sourceFactor)
	}
}

func TestAllowEvictsLeastRecentlyUsedInsteadOfRefusing(t *testing.T) {
	s, _, _ := setup(Options{TokenSlots: 2, Burst: 5})
	for _, k := range []string{"a", "b", "c"} {
		release, ok := s.allow(k)
		if !ok {
			t.Fatalf("slot %s refused", k)
		}
		release()
		for _, v := range s.tokens {
			v.updated = v.updated.Add(-time.Minute)
		}
	}
	h := sha256.Sum256([]byte("a"))
	if len(s.tokens) != 2 || s.tokens[hex.EncodeToString(h[:])] != nil {
		t.Fatal("least recently used slot not evicted")
	}
}

func TestRegistrationRejectsBodiesAndOtherMethods(t *testing.T) {
	s, _ := registering(Options{Registration: true, TrustedProxies: testProxy()})
	r := httptest.NewRequest("POST", "/v1/installations", strings.NewReader("x"))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("body accepted %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/v1/installations", nil)
	r.Header.Set("Authorization", "Bearer synthetic-test-token")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("GET registered %d", w.Code)
	}
	r = httptest.NewRequest("POST", lookup, nil)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("POST lookup answered %d", w.Code)
	}
}

func TestRegistrationLimitedPerWiderGroups(t *testing.T) {
	s, _ := registering(Options{Registration: true, RegisterPerHour: 100, RegisterNetworkPerHour: 2, RegisterWidePerHour: 3, TrustedProxies: testProxy()})
	// IPv6 /48: rotating /64s inside one /48 stops at its limit.
	registerToken(t, s, "2001:db8:1:1::1")
	registerToken(t, s, "2001:db8:1:2::1")
	if w := register(s, "2001:db8:1:3::1"); w.Code != 429 {
		t.Fatalf("/48 limit %d", w.Code)
	}
	// IPv6 /32: rotating /48s inside one /32 stops at its limit.
	registerToken(t, s, "2001:db8:2::1")
	if w := register(s, "2001:db8:3::1"); w.Code != 429 {
		t.Fatalf("/32 limit %d", w.Code)
	}
	// IPv4 /24: rotating addresses inside one /24.
	registerToken(t, s, "198.51.100.1")
	registerToken(t, s, "198.51.100.2")
	if w := register(s, "198.51.100.3"); w.Code != 429 {
		t.Fatalf("/24 limit %d", w.Code)
	}
	registerToken(t, s, "198.51.101.1") // next /24 is separate
}

// A flood that spends the shared budget leaves the reserved share for
// networks that have not registered this hour.
func TestRegistrationReservedForFreshNetworks(t *testing.T) {
	s, _ := registering(Options{Registration: true, RegisterPerHour: 100, RegisterNetworkPerHour: 100, RegisterGlobalPerHour: 6, RegisterReserved: 2, TrustedProxies: testProxy()})
	for i := 1; i <= 4; i++ {
		registerToken(t, s, "203.0.113."+string(rune('0'+i)))
	}
	if w := register(s, "203.0.113.9"); w.Code != 429 {
		t.Fatalf("flooding network used the reserve: %d", w.Code)
	}
	registerToken(t, s, "198.51.100.7")
	registerToken(t, s, "192.0.2.7")
	if w := register(s, "100.64.0.1"); w.Code != 429 {
		t.Fatalf("reserve not bounded %d", w.Code)
	}
}

func TestRegistrationFullOfProtectedAnswers429(t *testing.T) {
	s, st := registering(Options{Registration: true, TrustedProxies: testProxy()})
	st.full = "203.0.113.0/24"
	w := register(s, "203.0.113.50")
	if w.Code != 429 || !strings.Contains(w.Body.String(), "registry_full") || w.Header().Get("Retry-After") == "" {
		t.Fatalf("full registry %d %s", w.Code, w.Body.String())
	}
	registerToken(t, s, "198.51.100.50")
}

func TestRegisteredCredentialsScopedToLookupRoutes(t *testing.T) {
	if !registeredRoute("/v1/movies/tmdb/1") || !registeredRoute("/v1/music/cover/x") || !registeredRoute("/v1/search") || !registeredRoute("/v1/tmdb/movie/1") {
		t.Fatal("lookup route not in scope")
	}
	if registeredRoute("/v1/admin") || registeredRoute("/v1/lists/other") || registeredRoute("/v1/movies") {
		t.Fatal("unlisted route in registered scope")
	}
	s, _ := registering(Options{Registration: true, Burst: 100, TrustedProxies: testProxy()})
	tok := registerToken(t, s, "203.0.113.60")
	if w := reqFrom(s, "/v1/operator-only", tok, "203.0.113.60"); w.Code != 403 {
		t.Fatalf("registered reached an unlisted route: %d", w.Code)
	}
	if w := reqFrom(s, "/v1/operator-only", "synthetic-test-token", "203.0.113.60"); w.Code == 403 {
		t.Fatal("operator refused by scope")
	}
}

// Fetch-ahead never spends the last part of an installation's allowance:
// what a person opens next still gets through.
func TestAheadKeepsReserveForPeople(t *testing.T) {
	s, _, _ := setup(Options{Burst: 9, PerTokenConcurrent: 100})
	n := 0
	for i := 0; i < 20; i++ {
		if release, ok := s.allowFor("k", true); ok {
			release()
			n++
		}
	}
	if n == 0 || n >= 9 {
		t.Fatalf("ahead used %d of 9", n)
	}
	if release, ok := s.allowFor("k", false); !ok {
		t.Fatal("a person's request was refused after fetch-ahead")
	} else {
		release()
	}
}
