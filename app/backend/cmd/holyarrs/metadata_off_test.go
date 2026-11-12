package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// standInServer counts every request that reaches it; it answers 500, so
// a request that gets through is also visible as a failed lookup.
type standInServer struct {
	*httptest.Server
	hits  atomic.Int32
	mu    sync.Mutex
	paths map[string]int
	order []string
}

func newStandInServer(t *testing.T) *standInServer {
	t.Helper()
	s := &standInServer{paths: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.mu.Lock()
		s.paths[r.Method+" "+r.URL.Path]++
		s.order = append(s.order, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(s.Close)
	return s
}

// since lists the requests after the first n, in order.
func (s *standInServer) since(n int32) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(n) >= len(s.order) {
		return ""
	}
	return strings.Join(s.order[n:], "\n")
}

func (s *standInServer) seen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for p := range s.paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// metadataEnv points the app at the stand-in server, with no own key and
// no stand-in sources.
func metadataEnv(t *testing.T, srv *standInServer, mode string) {
	t.Helper()
	t.Setenv(metadata.EnvServiceURL, srv.URL)
	t.Setenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL", "true")
	t.Setenv(metadata.EnvMode, mode)
	for _, k := range []string{metadata.EnvTMDBKey, metadata.EnvTokenFile, "HOLYARRS_TMDB_URL", "HOLYARRS_TVMAZE_URL", "HOLYARRS_MUSICBRAINZ_URL",
		"HOLYARRS_DEEZER_URL", "HOLYARRS_DIRECT_SOURCES", "HOLYARRS_METADATA_SERVICE_TOKEN_FILE", metadata.EnvTVDBURL, metadata.EnvMDBListURL, metadata.EnvFanartURL} {
		t.Setenv(k, "")
	}
}

// ownerOf signs the first person up on a running app and hands back a
// client and its CSRF value.
func ownerOf(t *testing.T, base string) func(method, path, body string) (int, string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	csrf := ""
	call := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	st, body := call("POST", "/api/v1/auth/setup", `{"name":"owner","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &v)
	csrf = v.CSRF
	return call
}

// lookupPaths reach every kind of lookup: boot, Explore, search, title
// pages, music, trailers, people and the status page.
var lookupPaths = []string{
	"/api/v1/explore/head", "/api/v1/explore/tail?page=0", "/api/v1/explore/new", "/api/v1/explore/service-top?type=all&region=US", "/api/v1/discover?row=trending",
	"/api/v1/metadata/search?q=orchard", "/api/v1/search?q=orchard", "/api/v1/metadata/movie/603", "/api/v1/metadata/show/1399",
	"/api/v1/extras/movie/603", "/api/v1/wiki/movie/603", "/api/v1/trailer/movie/603", "/api/v1/shows/lookup?term=orchard",
	"/api/v1/artists/lookup?term=orchard", "/api/v1/explore/music", "/api/v1/music-trending?kind=artist&region=global", "/api/v1/music/artists/a74b1b7f-71a5-4011-9441-d0b5e4122711",
	"/api/v1/metadata/health", "/api/v1/settings/status", "/api/v1/settings/metadata",
}

// TestOffModeNeverReachesTheMetadataServer is the Off-mode proof: the whole
// app runs against a stand-in metadata server, and no request reaches it
// across start, the pages that look things up, the doctor and outside lists.
func TestOffModeNeverReachesTheMetadataServer(t *testing.T) {
	srv := newStandInServer(t)
	metadataEnv(t, srv, "off")
	dir := t.TempDir()
	_, addr := startApp(t, config{ConfigDir: dir}, runOpts{})
	call := ownerOf(t, "http://"+addr)
	for _, p := range lookupPaths {
		st, body := call("GET", p, "")
		if st == http.StatusServiceUnavailable || st == http.StatusBadRequest || strings.Contains(body, metadata.Down) {
			t.Errorf("%s: %d %s (off is not down)", p, st, body)
		}
	}
	if st, body := call("POST", "/api/v1/outside-lists", `{"name":"Kept","kind":"tmdb","ref":"42"}`); st < 300 {
		var l struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(body), &l)
		call("POST", "/api/v1/outside-lists/"+itoa(l.ID)+"/sync", "")
	}
	if st, body := call("GET", "/api/v1/settings/metadata", ""); st != 200 || !strings.Contains(body, `"mode":"off"`) || !strings.Contains(body, `"locked":true`) {
		t.Fatalf("settings: %d %s", st, body)
	}
	r := health.RunDoctor(context.Background(), health.DoctorOptions{ConfigDir: dir, MetadataURL: srv.URL})
	for _, c := range r.Checks {
		if c.Section == "Network" && strings.Contains(c.Text, "answered") {
			t.Errorf("doctor asked the server: %s", c.Text)
		}
	}
	// Background work (warmers, music packages, the trailer pass) gets a
	// moment to run.
	time.Sleep(3 * time.Second)
	if n := srv.hits.Load(); n != 0 {
		t.Fatalf("off mode sent %d requests to the metadata server:\n%s", n, srv.seen())
	}
}

// TestModeChangeAppliesWithoutRestart switches a running app to Off in
// Settings: the next lookups send nothing to the server.
func TestModeChangeAppliesWithoutRestart(t *testing.T) {
	srv := newStandInServer(t)
	metadataEnv(t, srv, "")
	_, addr := startApp(t, config{}, runOpts{})
	call := ownerOf(t, "http://"+addr)
	if st, body := call("PUT", "/api/v1/settings/metadata", `{"mode":"off"}`); st != 200 || !strings.Contains(body, `"mode":"off"`) {
		t.Fatalf("switch: %d %s", st, body)
	}
	// Requests already on their way when the switch landed get a moment.
	time.Sleep(500 * time.Millisecond)
	before := srv.hits.Load()
	for _, p := range lookupPaths {
		call("GET", p, "")
	}
	time.Sleep(time.Second)
	if n := srv.hits.Load() - before; n != 0 {
		t.Fatalf("after the switch %d requests reached the server:\n%s", n, srv.since(before))
	}
	if st, body := call("PUT", "/api/v1/settings/metadata", `{"mode":"full"}`); st != 200 || !strings.Contains(body, `"mode":"full"`) {
		t.Fatalf("back: %d %s", st, body)
	}
	call("GET", "/api/v1/metadata/search?q=orchard", "")
	if srv.hits.Load() == before {
		t.Fatal("back on the server, nothing was asked")
	}
}

// TestDockerModeTypoStartsWithLookupsOff: an unknown Docker value starts
// the app with lookups off, a red Health line naming the value, and the
// picker locked.
func TestDockerModeTypoStartsWithLookupsOff(t *testing.T) {
	srv := newStandInServer(t)
	metadataEnv(t, srv, "ful")
	_, addr := startApp(t, config{}, runOpts{})
	call := ownerOf(t, "http://"+addr)
	st, body := call("GET", "/api/v1/settings/status", "")
	if st != 200 || !strings.Contains(body, `\"ful\"`) || !strings.Contains(body, `"error"`) {
		t.Fatalf("status: %d %s", st, body)
	}
	if st, body := call("PUT", "/api/v1/settings/metadata", `{"mode":"full"}`); st != http.StatusConflict || !strings.Contains(body, "Set by your Docker settings") {
		t.Fatalf("locked picker: %d %s", st, body)
	}
	if n := srv.hits.Load(); n != 0 {
		t.Fatalf("a typo sent %d requests:\n%s", n, srv.seen())
	}
}

// Title art from fanart.tv passes through only in Your own keys mode; the
// server's own art is always on its picture store.
func TestFanartLinksOnlyWithOwnKeys(t *testing.T) {
	m := metadata.NewModes(func(string) string { return "" })
	a := &handlers{s: services{MetadataModes: m}}
	const u = "https://assets.fanart.tv/fanart/movies/603/hdmovielogo/y.png"
	if a.serverArtLink(u) != "" {
		t.Fatal("server mode passed a fanart.tv address")
	}
	_ = m.Set(metadata.ModeOwn)
	if a.serverArtLink(u) != u {
		t.Fatal("own keys dropped the fanart.tv picture")
	}
	if a.serverArtLink("https://evil.example/x.png") != "" {
		t.Fatal("another host passed")
	}
}
