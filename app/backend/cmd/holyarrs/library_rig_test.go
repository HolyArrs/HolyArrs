package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

func lbSetup(t *testing.T) (*httptest.Server, *database.Store, *jobs.Queue) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	hub := webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second})
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Live: hub, Metadata: metadata.Lab(), Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, q
}

func lbDo(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

// signedIn is one person's browser: cookies and the CSRF token.
// signedIn is one person's browser: cookies and the CSRF token.
type lbSignedIn struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
	id   int64
}

func (s *lbSignedIn) call(method, path, body string, want int) []byte {
	s.t.Helper()
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accounts.CSRFHeader, s.csrf)
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		s.t.Fatalf("%s %s: %d, want %d: %s", method, path, resp.StatusCode, want, b)
	}
	return b
}

type lbAskRig struct {
	srv   *httptest.Server
	st    *database.Store
	owner *lbSignedIn
	ana   *lbSignedIn
	bo    *lbSignedIn
	api   *handlers
}

// newAskRig runs the real API with sign-in: the owner luis and the members
// ana and bo (members may ask, nothing more).
// newAskRig runs the real API with sign-in: the owner luis and the members
// ana and bo (members may ask, nothing more).
func lbNewAskRig(t *testing.T) *lbAskRig {
	t.Helper()
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, lbInsertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(st, accounts.Options{Vault: v})
	md := &metadata.Static{Movies: []metadata.Movie{metadata.Lab().Movies[0],
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002"}},
		{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003"}}}}
	q := jobs.New(st, jobs.Options{Workers: 1})
	a := buildHandlers(services{Store: st, Queue: q, Metadata: md, Version: "test", Auth: au, Secrets: v, Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	r := &lbAskRig{srv: srv, st: st, api: a}

	browser := func() *lbSignedIn {
		jar, _ := cookiejar.New(nil)
		return &lbSignedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	r.owner = browser()
	r.owner.csrf, _ = read(r.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201))
	join := func(name string) *lbSignedIn {
		_, link := read(r.owner.call("POST", "/api/v1/users/invites", `{}`, 201))
		b := browser()
		b.csrf, _ = read(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201))
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	r.ana, r.bo = join("ana"), join("bo")
	return r
}

func lbInsertFolder(t *testing.T) func(tx *sql.Tx) error {
	dir := t.TempDir()
	return func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: dir, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	}
}

func lbAskOf(t *testing.T, b []byte) requests.View {
	t.Helper()
	var x struct {
		Ask requests.View `json:"ask"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x.Ask
}

type lbCtlFix struct {
	srv     *httptest.Server
	s       *database.Store
	pc      *parts.Control
	reg     *jobs.Registry
	api     *handlers
	runs    atomic.Int64 // passes of the test loops
	mu      sync.Mutex
	exits   []string
	exitErr error
}

// newCtl serves the API with a parts Control (one Movies loop, gates, and
// one core loop, backups) and records whole-app exits instead of exiting.
// newCtl serves the API with a parts Control (one Movies loop, gates, and
// one core loop, backups) and records whole-app exits instead of exiting.
func lbNewCtl(t *testing.T, withAuth bool) *lbCtlFix {
	t.Helper()
	return lbNewCtlMode(t, withAuth, false)
}

// newCtlMode is newCtl, in safe mode when safe is true.
// newCtlMode is newCtl, in safe mode when safe is true.
func lbNewCtlMode(t *testing.T, withAuth, safe bool) *lbCtlFix {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &lbCtlFix{s: s, reg: jobs.NewRegistry()}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	f.pc = &parts.Control{Store: s, Reg: f.reg, Jobs: q, Drain: time.Second, SafeMode: safe, Logf: func(string, ...any) {}}
	if err := f.pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	q.SetGate(f.pc)
	for _, name := range []string{"gates", "backups"} {
		f.pc.Add(jobs.Loop{Name: name, Label: name, Every: 20 * time.Millisecond, Run: func(context.Context) error { f.runs.Add(1); return nil }}, name == "gates")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); q.Stop(); f.reg.Wait() })
	f.pc.Start(ctx)
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	srvOpts := services{Store: s, Queue: q, Metadata: metadata.Lab(), Version: "test", Parts: f.pc, States: f.reg, SafeMode: safe,
		ExitApp: func(how string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.exitErr != nil {
				return f.exitErr
			}
			f.exits = append(f.exits, how)
			return nil
		}}
	if withAuth {
		srvOpts.Auth = accounts.New(s, accounts.Options{})
	}
	f.api = buildHandlers(srvOpts)
	f.srv = httptest.NewServer(f.api.Handler())
	t.Cleanup(f.srv.Close)
	return f
}

// confirmTwice runs a two-step control: the first call answers 202 with a
// confirm id and does nothing; the second, with the id, answers want.
// confirmTwice runs a two-step control: the first call answers 202 with a
// confirm id and does nothing; the second, with the id, answers want.
func lbConfirmTwice(t *testing.T, url string, want int) []byte {
	t.Helper()
	var ask struct {
		Confirm string `json:"confirm"`
		Expires int    `json:"expires_in_s"`
	}
	if err := json.Unmarshal(lbDo(t, "POST", url, "", 202), &ask); err != nil || ask.Confirm == "" || ask.Expires != 60 {
		t.Fatalf("first step: %+v %v", ask, err)
	}
	return lbDo(t, "POST", url, `{"confirm":"`+ask.Confirm+`"}`, want)
}

func lbDlServer(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test", Secrets: v}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// fakeSAB answers the version, queue and categories calls with one key.
// fakeSAB answers the version, queue and categories calls with one key.
func lbFakeSAB(t *testing.T, key string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("mode") == "version":
			_, _ = w.Write([]byte(`{"version":"5.1.3"}`))
		case q.Get("apikey") != key:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("API Key Incorrect"))
		case q.Get("mode") == "get_cats":
			_, _ = w.Write([]byte(`{"categories":["*","movies"]}`))
		default:
			_, _ = w.Write([]byte(`{"queue":{"status":"Idle","slots":[]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// keyCall sends one call with an API key in its header.
// keyCall sends one call with an API key in its header.
func lbKeyCall(t *testing.T, base, key, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
	req.Header.Set(accounts.APIKeyHeader, key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// kidEntry is one title the fake TMDB knows: id, name and US rating ("" for
// none).
// kidEntry is one title the fake TMDB knows: id, name and US rating ("" for
// none).
type lbKidEntry struct{ id, name, rating string }

var (
	lbKidMovies = []lbKidEntry{
		{"4101", "Gentle Meadow", "G"}, {"4102", "Paper Kite", "PG"}, {"4203", "Night Courier", "PG-13"},
		{"4204", "Iron Verdict", "R"}, {"4205", "Quiet Unknown", ""},
		{"4111", "Sunny Pond", "G"}, {"4211", "Dark Alley", "R"}, {"4212", "Nameless Reel", ""},
	}
	lbKidShows = []lbKidEntry{
		{"4121", "Harbor Friends", "TV-PG"}, {"4221", "Cold Precinct", "TV-MA"},
		{"4131", "Puppet Lane", "TV-Y"}, {"4231", "Blood Coast", "TV-MA"},
	}
	// blockedNames are every title a Kid limited to PG must never see.
	lbBlockedNames = []string{"Night Courier", "Iron Verdict", "Quiet Unknown", "Cold Precinct", "Loud Static",
		"Dark Alley", "Nameless Reel", "Blood Coast"}
	// blockedTMDB are their TMDB ids.
	lbBlockedTMDB = []string{"4203", "4204", "4205", "4221", "4211", "4212", "4231"}
)

// kidTMDB is a fake TMDB that knows the titles above with their US
// ratings (release_dates for movies, content_ratings for shows), a person
// credited in some of them, search, Explore rows and recommendations.
// kidTMDB is a fake TMDB that knows the titles above with their US
// ratings (release_dates for movies, content_ratings for shows), a person
// credited in some of them, search, Explore rows and recommendations.
type lbKidTMDB struct {
	srv     *httptest.Server
	details atomic.Int32 // movie and show details answered
	light   atomic.Int32 // release_dates and content_ratings answered
	// extra are more movies a test adds; trending, when set, replaces the
	// trending movies row. Both are set before the first request.
	extra    []lbKidEntry
	trending []string
}

func lbNewKidTMDB(t *testing.T) *lbKidTMDB {
	t.Helper()
	f := &lbKidTMDB{}
	find := func(list []lbKidEntry, id string) (lbKidEntry, bool) {
		for _, e := range list {
			if e.id == id {
				return e, true
			}
		}
		return lbKidEntry{}, false
	}
	movies := func() []lbKidEntry { return append(slices.Clone(lbKidMovies), f.extra...) }
	item := func(id string) string {
		if m, ok := find(movies(), id); ok {
			return fmt.Sprintf(`{"id":%s,"title":%q,"release_date":"2024-05-01","poster_path":"/p.jpg","genre_ids":[18]}`, m.id, m.name)
		}
		s, _ := find(lbKidShows, id)
		return fmt.Sprintf(`{"id":%s,"name":%q,"first_air_date":"2022-01-01","poster_path":"/p.jpg","genre_ids":[18]}`, s.id, s.name)
	}
	list := func(ids ...string) string {
		var out []string
		for _, id := range ids {
			out = append(out, item(id))
		}
		return `{"results":[` + strings.Join(out, ",") + `],"page":1,"total_pages":1,"total_results":` + fmt.Sprint(len(ids)) + `}`
	}
	movieRx := regexp.MustCompile(`^/3/movie/(\d+)(/credits|/recommendations|/release_dates)?$`)
	showRx := regexp.MustCompile(`^/3/tv/(\d+)(/season/1|/recommendations|/content_ratings)?$`)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch p {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = io.WriteString(w, `{"genres":[{"id":18,"name":"Drama"}]}`)
			return
		case "/3/trending/movie/week":
			if f.trending != nil {
				_, _ = io.WriteString(w, list(f.trending...))
				return
			}
			_, _ = io.WriteString(w, list("4211", "4111", "4212", "4204", "4101", "4203"))
			return
		case "/3/trending/tv/week":
			_, _ = io.WriteString(w, list("4231", "4131", "4221", "4121"))
			return
		case "/3/movie/popular", "/3/movie/upcoming", "/3/discover/movie":
			_, _ = io.WriteString(w, list("4211", "4111"))
			return
		case "/3/tv/popular", "/3/tv/on_the_air", "/3/discover/tv":
			_, _ = io.WriteString(w, list("4231", "4131"))
			return
		case "/3/search/movie":
			_, _ = io.WriteString(w, list("4111", "4211", "4212", "4203", "4101"))
			return
		case "/3/search/tv":
			_, _ = io.WriteString(w, list("4131", "4231", "4221", "4121"))
			return
		case "/3/search/person":
			_, _ = io.WriteString(w, `{"results":[{"id":4901,"name":"Ann Actor","known_for_department":"Acting","known_for":[`+
				item("4204")+`,`+item("4101")+`,`+item("4111")+`]}],"page":1,"total_pages":1,"total_results":1}`)
			return
		case "/3/person/4901":
			_, _ = io.WriteString(w, `{"id":4901,"name":"Ann Actor","known_for_department":"Acting"}`)
			return
		case "/3/person/4901/movie_credits":
			_, _ = io.WriteString(w, `{"cast":[{"id":4101,"title":"Gentle Meadow","release_date":"2024-05-01","character":"Hero"},
				{"id":4204,"title":"Iron Verdict","release_date":"2023-05-01","character":"Judge"},
				{"id":4111,"title":"Sunny Pond","release_date":"2022-05-01","character":"Friend"}],"crew":[]}`)
			return
		}
		if m := movieRx.FindStringSubmatch(p); m != nil {
			e, ok := find(movies(), m[1])
			if !ok {
				w.WriteHeader(404)
				return
			}
			switch m[2] {
			case "/credits":
				_, _ = io.WriteString(w, `{"cast":[{"id":4901,"name":"Ann Actor","character":"Hero"}],"crew":[]}`)
			case "/recommendations":
				_, _ = io.WriteString(w, list("4111", "4211", "4203"))
			case "/release_dates":
				f.light.Add(1)
				_, _ = io.WriteString(w, `{"id":`+e.id+`,"results":[{"iso_3166_1":"US","release_dates":[{"type":3,"certification":"`+e.rating+`"}]}]}`)
			default:
				f.details.Add(1)
				cert := ""
				if e.rating != "" {
					cert = `,"release_dates":{"results":[{"iso_3166_1":"GB","release_dates":[{"type":3,"certification":"U"}]},` +
						`{"iso_3166_1":"US","release_dates":[{"type":3,"certification":"` + e.rating + `","release_date":"2024-05-01T00:00:00.000Z"}]}]}`
				}
				_, _ = fmt.Fprintf(w, `{"id":%s,"title":%q,"release_date":"2024-05-01","runtime":90,"genres":[{"name":"Drama"}]%s,
					"credits":{"cast":[{"name":"Ann Actor","character":"Hero"}]},"recommendations":%s}`, e.id, e.name, cert, list("4111", "4211", "4203"))
			}
			return
		}
		if m := showRx.FindStringSubmatch(p); m != nil {
			e, ok := find(lbKidShows, m[1])
			if !ok {
				w.WriteHeader(404)
				return
			}
			switch m[2] {
			case "/season/1":
				_, _ = io.WriteString(w, `{"episodes":[{"id":1,"season_number":1,"episode_number":1,"name":"Pilot","air_date":"2022-01-01"}]}`)
			case "/recommendations":
				_, _ = io.WriteString(w, list("4131", "4231"))
			case "/content_ratings":
				f.light.Add(1)
				_, _ = io.WriteString(w, `{"id":`+e.id+`,"results":[{"iso_3166_1":"US","rating":"`+e.rating+`"}]}`)
			default:
				f.details.Add(1)
				rating := ""
				if e.rating != "" {
					rating = `,"content_ratings":{"results":[{"iso_3166_1":"US","rating":"` + e.rating + `"}]}`
				}
				_, _ = fmt.Fprintf(w, `{"id":%s,"name":%q,"first_air_date":"2022-01-01","status":"Returning Series",
					"seasons":[{"season_number":1,"episode_count":1,"air_date":"2022-01-01"}]%s,"recommendations":%s}`, e.id, e.name, rating, list("4131", "4231"))
			}
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *lbKidTMDB) provider() *metadata.TMDB {
	return &metadata.TMDB{BaseURL: f.srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

type lbKidRig struct {
	srv   *httptest.Server
	st    *database.Store
	tmdb  *lbKidTMDB
	owner *lbSignedIn
	ana   *lbSignedIn // a member without a limit
	kai   *lbSignedIn // a Kid limited to PG
	api   *handlers
	ids   map[string]int64
}

// blockedIDs are the Library ids of the titles above PG (and the artist);
// they are moved to 9101 and up so a body check cannot mistake them.
// blockedIDs are the Library ids of the titles above PG (and the artist);
// they are moved to 9101 and up so a body check cannot mistake them.
var lbBlockedIDs = []int64{9101, 9102, 9103, 9104, 9105}

// newKidRig runs the real API with sign-in and a fake TMDB: the owner luis,
// the member ana and the Kid kai (PG). The Library holds movies rated G, PG,
// PG-13, R and none, a TV-PG show, a TV-MA show and an artist.
// newKidRig runs the real API with sign-in and a fake TMDB: the owner luis,
// the member ana and the Kid kai (PG). The Library holds movies rated G, PG,
// PG-13, R and none, a TV-PG show, a TV-MA show and an artist.
func lbNewKidRig(t *testing.T, off ...parts.Part) *lbKidRig {
	t.Helper()
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, lbInsertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	f := lbNewKidTMDB(t)
	prov := f.provider()
	q := jobs.New(st, jobs.Options{Workers: 1})
	hub := webserver.NewLiveHub(webserver.LiveOptions{})
	lib := &library.Service{Store: st, Queue: q, Metadata: prov, Live: hub, Shows: metadata.ShowSources{"tmdb": prov}}
	var pc *parts.Control
	if len(off) > 0 {
		pc = &parts.Control{Store: st, Reg: jobs.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
		if err := pc.Load(ctx); err != nil {
			t.Fatal(err)
		}
		for _, part := range off {
			if err := pc.Set(ctx, part, false, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}
	a := buildHandlers(services{Store: st, Queue: q, Metadata: prov, Version: "test", Auth: accounts.New(st, accounts.Options{Vault: v}), Secrets: v, Live: hub, Library: lib,
		Parts: pc})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	k := &lbKidRig{srv: srv, st: st, tmdb: f, ids: map[string]int64{}, api: a}
	browser := func() *lbSignedIn {
		jar, _ := cookiejar.New(nil)
		return &lbSignedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	k.owner = browser()
	k.owner.csrf, _ = read(k.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201))
	join := func(name, invite string) *lbSignedIn {
		_, link := read(k.owner.call("POST", "/api/v1/users/invites", invite, 201))
		b := browser()
		b.csrf, _ = read(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201))
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	k.ana = join("ana", `{}`)
	k.kai = join("kai", `{"max_rating":"PG"}`)

	soon := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 2)
	k.add(t, "movie", "Gentle Meadow", "4101", "G", 0, &soon)
	k.add(t, "movie", "Paper Kite", "4102", "PG", 0, nil)
	k.add(t, "show", "Harbor Friends", "4121", "TV-PG", 0, nil)
	k.add(t, "movie", "Night Courier", "4203", "PG-13", 9101, nil)
	k.add(t, "movie", "Iron Verdict", "4204", "R", 9102, &soon)
	k.add(t, "movie", "Quiet Unknown", "4205", "", 9103, nil)
	k.add(t, "show", "Cold Precinct", "4221", "TV-MA", 9104, nil)
	k.add(t, "artist", "Loud Static", "", "", 9105, nil)
	err = st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO movie_collections(id, source_id, name) VALUES (1, 'tmdb:51', 'Shelf One'), (2, 'tmdb:52', 'Verdict Saga')"); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (1, ?), (1, 9102), (2, 9102)", k.ids["Gentle Meadow"])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// add puts one title in the Library; a movie also gets a missing copy
// (Home's "missing" list). id moves the title to that id when not 0.
// add puts one title in the Library; a movie also gets a missing copy
// (Home's "missing" list). id moves the title to that id when not 0.
func (k *lbKidRig) add(t *testing.T, kind, name, tmdbID, rating string, id int64, cinema *time.Time) {
	t.Helper()
	err := k.st.Write(context.Background(), func(tx *sql.Tx) error {
		ids := map[string]string{"tmdb": tmdbID}
		src := "tmdb"
		if kind == "artist" {
			ids, src = map[string]string{"musicbrainz": "5b11f4ce-a62d-471e-81fc-a69a8278c7da"}, "musicbrainz"
		}
		title := mediafiles.Title{Kind: kind, Name: name, SortName: strings.ToLower(name), SourceIDs: ids, MetadataSource: src, Rating: rating,
			WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released",
			CinemaDate: cinema, KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		if id != 0 {
			if _, err := tx.Exec("UPDATE titles SET id = ? WHERE id = ?", id, title.ID); err != nil {
				return err
			}
			title.ID = id
		}
		k.ids[name] = title.ID
		if kind != "movie" {
			return nil
		}
		var rulesID, folderID int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rulesID); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders WHERE kind = 'movie' ORDER BY id LIMIT 1").Scan(&folderID); err != nil {
			return err
		}
		item := mediafiles.Item{TitleID: title.ID, Kind: "movie", Name: &title.Name}
		if _, err := database.Insert(tx, "items", &item, domain.System); err != nil {
			return err
		}
		version := mediafiles.Version{TitleID: title.ID, Label: "Main", IsMain: true, LibraryFolderID: folderID, PickRulesID: rulesID}
		if _, err := database.Insert(tx, "versions", &version, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: item.ID, VersionID: version.ID, State: mediafiles.SlotWanted}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

var lbDigitsAround = regexp.MustCompile(`\d`)

// mentionsBlocked names the first blocked title body mentions: a name, a
// Library id or a TMDB id (ids only when no digit touches them).
// mentionsBlocked names the first blocked title body mentions: a name, a
// Library id or a TMDB id (ids only when no digit touches them).
func lbMentionsBlocked(body string) string {
	for _, n := range lbBlockedNames {
		if strings.Contains(body, n) {
			return n
		}
	}
	ids := append([]string{}, lbBlockedTMDB...)
	for _, id := range lbBlockedIDs {
		ids = append(ids, fmt.Sprint(id))
	}
	for _, id := range ids {
		for i := strings.Index(body, id); i >= 0; {
			before := i == 0 || !lbDigitsAround.MatchString(body[i-1:i])
			end := i + len(id)
			after := end == len(body) || !lbDigitsAround.MatchString(body[end:end+1])
			if before && after {
				return id
			}
			next := strings.Index(body[i+1:], id)
			if next < 0 {
				break
			}
			i += 1 + next
		}
	}
	return ""
}

func lbSameNames(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s: got %q, want %q", what, got, want)
	}
}

// fakeRadarr answers the few Radarr calls the read makes (written from the
// Radarr manual), with secrets that must never come back.
// fakeRadarr answers the few Radarr calls the read makes (written from the
// Radarr manual), with secrets that must never come back.
func lbFakeRadarr(t *testing.T, key string) string {
	t.Helper()
	routes := map[string]string{
		"/api/v3/system/status": `{"appName":"Radarr","version":"6.4.4","migrationVersion":242}`,
		"/api/v3/movie":         `[{"id":1,"tmdbId":603,"title":"The Matrix","year":1999,"path":"/movies/The Matrix (1999)","monitored":true,"qualityProfileId":1,"tags":[]}]`,
		"/api/v3/indexer":       `[{"id":1,"name":"Nyaa","implementation":"Torznab","fields":[{"name":"baseUrl","value":"http://x/"},{"name":"apiKey","value":"SECRET_IN_RADARR"}]}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, ok := routes[r.URL.Path]
		if !ok {
			b = "[]"
			if strings.HasSuffix(r.URL.Path, "/naming") {
				b = "{}"
			}
		}
		_, _ = w.Write([]byte(b))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func lbReadBody(key, url string) string {
	b, _ := json.Marshal(map[string]any{"kind": "radarr", "url": url, "key": key, "allow_local_network": true})
	return string(b)
}

type lbPickRulesDetail struct {
	ID       int64                    `json:"id"`
	Name     string                   `json:"name"`
	Kinds    []string                 `json:"kinds"`
	Preset   bool                     `json:"preset"`
	Revision int                      `json:"revision"`
	Body     mediafiles.PickRulesBody `json:"body"`
	InUse    struct {
		Versions       int `json:"versions"`
		LibraryFolders int `json:"library_folders"`
		OutsideLists   int `json:"outside_lists"`
		Wishlist       int `json:"wishlist"`
		TitleDefaults  int `json:"title_defaults"`
	} `json:"in_use"`
}

func lbRuleJSON(t *testing.T, name string, kinds []string, b mediafiles.PickRulesBody) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"name": name, "kinds": kinds, "body": b})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func lbDecodeJSON(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
}

func lbDay(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

// libSetup is an API with two TMDB-like movies, no Library folder yet and a
// running queue with the library's handlers and a fake search.
// libSetup is an API with two TMDB-like movies, no Library folder yet and a
// running queue with the library's handlers and a fake search.
func lbLibSetup(t *testing.T) (*httptest.Server, *database.Store, chan int64) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	md := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134", "imdb": "tt15239678"},
			CinemaDate: lbDay("2024-03-01"), DigitalDate: lbDay("2024-04-16")},
		{Name: "The Lantern Keepers", Year: 2031, IDs: map[string]string{"tmdb": "900001"}, CinemaDate: lbDay("2031-02-01")},
	}}
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond})
	svc := &library.Service{Store: s, Queue: q, Metadata: md}
	svc.Register(q)
	searched := make(chan int64, 10)
	q.Handle(library.KindSearchSlot, func(ctx context.Context, r *jobs.Run) error { searched <- r.Job.Subject.ID; return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Metadata: md, Version: "test", Library: svc}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, searched
}

func lbItoa(n int64) string { return strconv.FormatInt(n, 10) }
