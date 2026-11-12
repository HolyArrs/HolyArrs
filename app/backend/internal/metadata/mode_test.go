package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestParseModeLoose(t *testing.T) {
	for raw, want := range map[string]Mode{"full": ModeServer, " FULL ": ModeServer, "server": ModeServer, "own": ModeOwn, "Own-Keys": ModeOwn, "own_keys": ModeOwn, "off": ModeOff, "OFF": ModeOff, "none": ModeOff} {
		if got, ok := ParseMode(raw); !ok || got != want {
			t.Errorf("%q: %q %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"ful", "", "yes please"} {
		if got, ok := ParseMode(raw); ok || got != ModeOff {
			t.Errorf("%q: %q %v, want off and not understood", raw, got, ok)
		}
	}
}

func TestModesFromDocker(t *testing.T) {
	m := NewModes(envOf(nil))
	if m.Current() != ModeServer || m.Locked() || m.BadValue() != "" {
		t.Fatalf("no Docker value: %q locked=%v", m.Current(), m.Locked())
	}
	if err := m.Set(ModeOff); err != nil || m.Current() != ModeOff {
		t.Fatalf("set: %v %q", err, m.Current())
	}
	typo := NewModes(envOf(map[string]string{EnvMode: "ful"}))
	if typo.Current() != ModeOff || typo.BadValue() != "ful" || !typo.Locked() {
		t.Fatalf("a typo is off and named: %q %q", typo.Current(), typo.BadValue())
	}
	if err := typo.Set(ModeServer); !errors.Is(err, ErrModeLocked) || typo.Current() != ModeOff {
		t.Fatalf("Docker locks the picker: %v", err)
	}
	urlOff := NewModes(envOf(map[string]string{EnvServiceURL: "off"}))
	if urlOff.Current() != ModeOwn || !urlOff.Locked() {
		t.Fatalf("URL=off means own keys: %q", urlOff.Current())
	}
	both := NewModes(envOf(map[string]string{EnvServiceURL: "off", EnvMode: "off"}))
	if both.Current() != ModeOff {
		t.Fatalf("the mode wins over URL=off: %q", both.Current())
	}
	var none *Modes
	if none.Current() != ModeServer || !none.Server() {
		t.Fatal("a nil Modes is the metadata server")
	}
}

func TestServiceURLOffKeepsDefault(t *testing.T) {
	t.Setenv(EnvServiceURL, "off")
	if ServiceURL() != DefaultServiceURL {
		t.Fatalf("got %q", ServiceURL())
	}
}

func TestGateSendsNothingUnlessServerMode(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	m := NewModes(envOf(nil))
	g := &Gate{Next: http.DefaultTransport, Modes: m}
	cl := &http.Client{Transport: g}
	for _, mode := range []Mode{ModeOff, ModeOwn} {
		_ = m.Set(mode)
		_, err := cl.Get(srv.URL + "/v1/search")
		if !errors.Is(err, ErrSourceOff) {
			t.Fatalf("%s: %v", mode, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("%d requests reached the server", hits.Load())
	}
	_ = m.Set(ModeServer)
	if resp, err := cl.Get(srv.URL + "/v1/search"); err != nil {
		t.Fatal(err)
	} else {
		_ = resp.Body.Close()
	}
	if hits.Load() != 1 {
		t.Fatalf("server mode sends: %d", hits.Load())
	}
}

func TestGuardAllowsOnlyItsMode(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	m := NewModes(envOf(nil))
	cl := &http.Client{Transport: m.Guard(nil, OwnOnly)}
	for _, mode := range []Mode{ModeServer, ModeOff} {
		_ = m.Set(mode)
		if _, err := cl.Get(srv.URL); !errors.Is(err, ErrSourceOff) {
			t.Fatalf("%s: %v", mode, err)
		}
	}
	_ = m.Set(ModeOwn)
	resp, err := cl.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

// hostedOn is a Hosted against a stand-in metadata server, with no own key
// from the environment.
func hostedOn(t *testing.T, srv *httptest.Server) *Hosted {
	t.Helper()
	t.Setenv(EnvServiceURL, srv.URL)
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	t.Setenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE", "")
	t.Setenv(EnvTMDBKey, "")
	t.Setenv(EnvTokenFile, "")
	t.Setenv("HOLYARRS_TMDB_URL", "")
	t.Setenv(EnvMode, "")
	h, err := NewHosted(testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestModeChangeAppliesAtOnceAndKeepsCopies(t *testing.T) {
	var registered, tmdb atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/installations":
			registered.Add(1)
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"schema_version":1,"` + "tok" + `en":"` + fixtureInstall + `"}`))
		case r.URL.Path == "/v1/tmdb/movie/603":
			tmdb.Add(1)
			_, _ = w.Write([]byte(`{"id":603,"title":"Fixture Film","release_date":"1999-03-31"}`))
		default:
			tmdb.Add(1)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	h := hostedOn(t, srv)
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h.Disk = LookupCache{Store: st}
	ctx := context.Background()
	tm := h.TMDB()
	before, err := tm.Movie(ctx, "tmdb:603")
	if err != nil {
		t.Fatal(err)
	}
	sent := registered.Load() + tmdb.Load()
	if err := h.Modes.Set(ModeOff); err != nil {
		t.Fatal(err)
	}
	// A fresh client (no memory copy): the copy on disk still answers.
	again, err := h.TMDB().Movie(ctx, "tmdb:603")
	if err != nil || again.Name != before.Name {
		t.Fatalf("kept copy while off: %v %v", again, err)
	}
	if _, err := h.TMDB().Search(ctx, "Never Asked", 0); !SourceOff(err) {
		t.Fatalf("a new lookup while off: %v", err)
	}
	if _, err := h.Service.TrailerPick(ctx, "movie", "603", "en"); !SourceOff(err) {
		t.Fatalf("server call while off: %v", err)
	}
	if registered.Load()+tmdb.Load() != sent {
		t.Fatalf("off mode reached the server: %d requests", registered.Load()+tmdb.Load()-sent)
	}
	_ = h.Modes.Set(ModeServer)
	if back, err := h.TMDB().Movie(ctx, "tmdb:603"); err != nil || back.Name != before.Name {
		t.Fatalf("back on the server: %v %v", back, err)
	}
}

func TestOwnKeysAsksTMDBDirectly(t *testing.T) {
	var registered, server atomic.Int32
	srv := fakeMetadataServer(t, &registered, &server)
	var direct atomic.Int32
	tmdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer own-tmdb-fixture" {
			w.WriteHeader(401)
			return
		}
		direct.Add(1)
		_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"total_results":1,"results":[{"id":604,"title":"Own Film","release_date":"2003-05-15"}]}`))
	}))
	defer tmdbSrv.Close()
	h := hostedOn(t, srv)
	h.stand = tmdbSrv.URL + "/3"
	h.Direct = directClient(h.Modes, true)
	ctx := context.Background()
	_ = h.Modes.Set(ModeOwn)
	if _, err := h.TMDB().Search(ctx, "Own", 0); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("no key yet: %v", err)
	}
	if err := h.Vault.SetSetting(ctx, KeyGroup, KeyTMDB, "own-tmdb-fixture", database.System); err != nil {
		t.Fatal(err)
	}
	ms, err := h.TMDB().Search(ctx, "Own", 0)
	if err != nil || len(ms) != 1 || ms[0].Name != "Own Film" {
		t.Fatalf("%v %v", ms, err)
	}
	if direct.Load() != 1 || registered.Load()+server.Load() != 0 {
		t.Fatalf("direct %d, server %d", direct.Load(), registered.Load()+server.Load())
	}
	if !strings.HasPrefix(h.TMDBRoute(ctx).Base, tmdbSrv.URL) {
		t.Fatalf("route %q", h.TMDBRoute(ctx).Base)
	}
}
