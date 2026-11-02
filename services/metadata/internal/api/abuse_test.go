package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/stats"
)

func reqFrom(s *Server, path, token, ip string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if ip != "" {
		r.Header.Set("CF-Connecting-IP", ip)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestClientAddressLimitAppliesBeforeAuthorization(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, PerMinute: 100000, ClientPerMinute: 1, ClientBurst: 3, TrustedProxies: testProxy()})
	for range 3 {
		if w := reqFrom(s, lookup, "wrong", "203.0.113.5"); w.Code != 401 {
			t.Fatalf("status %d", w.Code)
		}
	}
	w := reqFrom(s, lookup, "synthetic-test-token", "203.0.113.5")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("address limit %d", w.Code)
	}
	if w := reqFrom(s, lookup, "synthetic-test-token", "203.0.113.6"); w.Code != 200 {
		t.Fatalf("other address %d", w.Code)
	}
	if w := reqFrom(s, "/healthz", "", "203.0.113.5"); w.Code != 200 {
		t.Fatalf("health limited %d", w.Code)
	}
}

func TestRequestBodyAndOversizedHeadersRejected(t *testing.T) {
	s, _, _ := setup(Options{})
	r := httptest.NewRequest("GET", lookup, nil)
	r.Header.Set("Authorization", "Bearer synthetic-test-token")
	r.ContentLength = 10
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("body accepted %d", w.Code)
	}
}

func TestStatsRequireTokenAndCountCache(t *testing.T) {
	rec, err := stats.Open(filepath.Join(t.TempDir(), "stats.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	s, _, _ := setup(Options{Burst: 100, Stats: rec, StatsToken: syntheticReader})
	for range 3 {
		if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if w := req(s, "/v1/stats", ""); w.Code != 401 {
		t.Fatalf("anonymous stats %d", w.Code)
	}
	if w := req(s, "/v1/stats", "synthetic-test-token"); w.Code != 401 {
		t.Fatalf("installation token read stats %d", w.Code)
	}
	w := req(s, "/v1/stats?window_hours=1", syntheticReader)
	if w.Code != 200 {
		t.Fatalf("stats %d %s", w.Code, w.Body.String())
	}
	var body stats.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Totals.CacheHits != 2 || body.Totals.CacheMisses != 1 || body.Totals.Requests != 3 || body.ItemsStored["movies"] != 1 || body.WindowSeconds != 3600 {
		t.Fatalf("stats body %+v", body)
	}
	if req(s, "/v1/stats?window_hours=abc", syntheticReader).Code != 400 {
		t.Fatal("bad window accepted")
	}
}

func TestStatsDisabledWithoutToken(t *testing.T) {
	s, _, _ := setup(Options{})
	if w := req(s, "/v1/stats", ""); w.Code != 401 && w.Code != 404 {
		t.Fatalf("stats without config %d", w.Code)
	}
	_ = time.Second
}

var syntheticReader = "synthetic-" + "dashboard-reader"

func TestForwardedAddressTrustedOnlyFromProxy(t *testing.T) {
	_, proxy, _ := net.ParseCIDR("172.18.0.0/16")
	s, _, _ := setup(Options{Burst: 100, PerMinute: 100000, ClientPerMinute: 1, ClientBurst: 1, ClientAuthPerMinute: 1, ClientAuthBurst: 1, TrustedProxies: []*net.IPNet{proxy}})
	spoof := func(remote, header string) int {
		r := httptest.NewRequest("GET", lookup, nil)
		r.RemoteAddr = remote
		r.Header.Set("Authorization", "Bearer synthetic-test-token")
		r.Header.Set("CF-Connecting-IP", header)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}
	if spoof("198.51.100.9:1", "203.0.113.1") != 200 || spoof("198.51.100.9:1", "203.0.113.2") != 429 {
		t.Fatal("untrusted peer could choose its address")
	}
	if spoof("172.18.0.3:1", "203.0.113.1") != 200 || spoof("172.18.0.3:1", "203.0.113.2") != 200 || spoof("172.18.0.3:1", "203.0.113.2") != 429 {
		t.Fatal("trusted proxy address not used")
	}
}

func TestFullClientTableEvictsInsteadOfRejecting(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, PerMinute: 100000, TokenSlots: 2})
	for _, ip := range []string{"192.0.2.1:1", "192.0.2.2:1", "192.0.2.3:1", "192.0.2.4:1"} {
		r := httptest.NewRequest("GET", lookup, nil)
		r.RemoteAddr = ip
		r.Header.Set("Authorization", "Bearer synthetic-test-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s rejected %d", ip, w.Code)
		}
	}
	if len(s.clients) > 2 {
		t.Fatalf("table grew to %d", len(s.clients))
	}
}

func testProxy() []*net.IPNet {
	_, n, _ := net.ParseCIDR("192.0.2.0/24")
	return []*net.IPNet{n}
}

func TestStatsItemsListRecentAdditions(t *testing.T) {
	rec, err := stats.Open(filepath.Join(t.TempDir(), "stats.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	s, _, _ := setup(Options{Burst: 100, Stats: rec, StatsToken: syntheticReader})
	if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/v1/stats/items", "/v1/stats/items?kind=movies"} {
		if w := req(s, path, ""); w.Code != 401 {
			t.Fatalf("anonymous items %d", w.Code)
		}
		if w := req(s, path, "synthetic-test-token"); w.Code != 401 {
			t.Fatalf("installation token read items %d", w.Code)
		}
	}
	for _, bad := range []string{"kind=book", "limit=0", "limit=101", "offset=-1", "window_hours=200", "range=24h", "kind=movies&kind=shows"} {
		if w := req(s, "/v1/stats/items?"+bad, syntheticReader); w.Code != 400 {
			t.Fatalf("%s accepted %d", bad, w.Code)
		}
	}
	w := req(s, "/v1/stats/items?window_hours=24&kind=movies&limit=10", syntheticReader)
	if w.Code != 200 {
		t.Fatalf("items %d %s", w.Code, w.Body.String())
	}
	var page stats.ItemPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Kind != "movies" || page.Items[0].Provider != "tmdb" || page.Items[0].AddedAt == "" || page.Limit != 10 {
		t.Fatalf("items body %+v", page)
	}
	w = req(s, "/v1/stats/items?kind=music", syntheticReader)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("music items %+v", page)
	}
}

func TestStatsCarrySpeedSection(t *testing.T) {
	rec, err := stats.Open(filepath.Join(t.TempDir(), "stats.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	s, _, _ := setup(Options{Burst: 100, Stats: rec, StatsToken: syntheticReader, StatusExtra: func(context.Context, time.Duration) map[string]any {
		return map[string]any{"resources": map[string]int{"disk_free_bytes": 5}}
	}})
	w := req(s, "/v1/stats", syntheticReader)
	var body struct {
		stats.Snapshot
		Speed map[string]map[string]int `json:"speed"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Speed["resources"]["disk_free_bytes"] != 5 || body.WindowSeconds == 0 {
		t.Fatalf("speed section %d %s", w.Code, w.Body)
	}
}

// A working credential gets the larger address limit; failed credentials
// spend the ordinary one, and an address that spent it is refused before
// any credential lookup.
func TestCredentialedReadsGetTheLargerAddressLimit(t *testing.T) {
	s, m, _ := setup(Options{Burst: 1000, PerMinute: 100000, ClientPerMinute: 1, ClientBurst: 2, ClientAuthPerMinute: 1, ClientAuthBurst: 8})
	ask := func(token string) int {
		r := httptest.NewRequest("GET", lookup, nil)
		r.RemoteAddr = "198.51.100.7:1"
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}
	for i := range 8 {
		if c := ask("synthetic-test-token"); c != 200 {
			t.Fatalf("credentialed read %d: %d", i, c)
		}
	}
	if c := ask("synthetic-test-token"); c != 429 {
		t.Fatalf("past the credentialed burst: %d", c)
	}
	// Anonymous reads keep the ordinary limit (2).
	if ask("") != 401 || ask("") != 401 || ask("") != 429 {
		t.Fatal("anonymous limit not kept")
	}
	// Failed credentials: the ordinary bucket is spent, so the next bearer
	// read is refused before its lookup.
	m.authorized = false
	s2, m2, _ := setup(Options{Burst: 1000, PerMinute: 100000, ClientPerMinute: 1, ClientBurst: 2, ClientAuthBurst: 100})
	m2.authorized = false
	s = s2
	if ask("bad") != 401 || ask("bad") != 401 || ask("bad") != 429 {
		t.Fatal("failed credentials not limited")
	}
	m2.authorized = true
	if c := ask("synthetic-test-token"); c != 429 {
		t.Fatalf("an address that spent its limit on bad credentials got through: %d", c)
	}
}

// /metrics (and the statistics) check the statistics token, not an install
// credential: a wrong bearer there is charged to the ordinary address
// limit and runs out at its burst.
func TestMetricsWrongBearerIsLimited(t *testing.T) {
	rec, err := stats.Open(filepath.Join(t.TempDir(), "s.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	s, _, _ := setup(Options{Burst: 1000, PerMinute: 100000, Stats: rec, StatsToken: syntheticReader, ClientPerMinute: 1, ClientBurst: 3, ClientAuthBurst: 100})
	codes := []int{}
	for _, path := range []string{"/metrics", "/metrics", "/v1/stats", "/metrics"} {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "198.51.100.8:1"
		r.Header.Set("Authorization", "Bearer guess-guess-guess-guess-guess")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		codes = append(codes, w.Code)
	}
	if codes[0] != 401 || codes[1] != 401 || codes[2] != 401 || codes[3] != 429 {
		t.Fatalf("wrong statistics bearers: %v", codes)
	}
}

// Many bad bearers at once from one address: each takes from the ordinary
// limit before its lookup, so no more than the burst get a lookup.
func TestConcurrentBadBearersLimited(t *testing.T) {
	s, m, _ := setup(Options{Burst: 1000, PerMinute: 100000, ClientPerMinute: 1, ClientBurst: 5, ClientAuthBurst: 100})
	m.authorized = false
	var mu sync.Mutex
	got := map[int]int{}
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("GET", lookup, nil)
			r.RemoteAddr = "198.51.100.9:1"
			r.Header.Set("Authorization", "Bearer bad-guess")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			mu.Lock()
			got[w.Code]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if got[401] != 5 || got[429] != 45 {
		t.Fatalf("codes %v, want 5 lookups then 429", got)
	}
	// A good credential from a fresh address still gets the larger burst.
	m.authorized = true
	for i := range 20 {
		r := httptest.NewRequest("GET", lookup, nil)
		r.RemoteAddr = "198.51.100.10:1"
		r.Header.Set("Authorization", "Bearer synthetic-test-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("good read %d past the ordinary burst: %d", i, w.Code)
		}
	}
}
