package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type discoverTMDB struct {
	srv     *httptest.Server
	hits    atomic.Int32
	limited atomic.Bool
	// details counts asks for movie 11's details only: background work the
	// first page starts asks for other things and lands in hits at any time.
	details atomic.Int32
}

func newDiscoverTMDB(t *testing.T) *discoverTMDB {
	t.Helper()
	f := &discoverTMDB{}
	list := func(ids ...int) string {
		var out []string
		for _, id := range ids {
			out = append(out, fmt.Sprintf(`{"id":%d,"title":"M%d","name":"S%d","release_date":"2024-05-01","first_air_date":"2022-01-01","poster_path":"/p.jpg","genre_ids":[18]}`, id, id, id))
		}
		return `{"results":[` + strings.Join(out, ",") + `]}`
	}
	many := make([]int, 30)
	for i := range many {
		many[i] = 100 + i
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if f.limited.Load() {
			w.WriteHeader(429)
			return
		}
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"}]}`))
		case "/3/trending/movie/week":
			_, _ = w.Write([]byte(list(append([]int{11, 12, 13}, many...)...)))
		case "/3/trending/tv/week":
			_, _ = w.Write([]byte(list(21, 22)))
		case "/3/movie/popular", "/3/movie/upcoming", "/3/tv/popular", "/3/tv/on_the_air", "/3/discover/movie", "/3/discover/tv":
			_, _ = w.Write([]byte(list(31)))
		case "/3/watch/providers/movie":
			_, _ = w.Write([]byte(`{"results":[{"provider_id":8,"provider_name":"Netflix","logo_path":"/n.jpg","display_priority":5,"display_priorities":{"US":1}},{"provider_id":337,"provider_name":"Disney Plus","logo_path":"/d.jpg","display_priorities":{"US":2}}]}`))
		case "/3/watch/providers/tv":
			_, _ = w.Write([]byte(`{"results":[{"provider_id":15,"provider_name":"Hulu","display_priorities":{"US":0}},{"provider_id":8,"provider_name":"Netflix","display_priorities":{"US":3}}]}`))
		case "/3/movie/11/recommendations":
			_, _ = w.Write([]byte(list(41, 12)))
		case "/3/movie/11":
			f.details.Add(1)
			_, _ = w.Write([]byte(`{"id":11,"title":"M11","release_date":"2024-05-01","runtime":100,"genres":[{"name":"Drama"}],
				"credits":{"cast":[{"name":"Ann","character":"Hero"}],"crew":[{"id":5,"name":"Dee","job":"Director"}]},"keywords":{"keywords":[{"name":"heist"}]},"belongs_to_collection":{"id":90,"name":"Set"},"recommendations":` + list(12, 13, 41) + `}`))
		case "/3/tv/21/watch/providers":
			_, _ = w.Write([]byte(`{"results":{"DE":{"flatrate":[{"provider_id":8,"provider_name":"Netflix"}]}}}`))
		case "/3/collection/90":
			_, _ = w.Write([]byte(`{"name":"The Set","parts":[{"id":11,"title":"M11","release_date":"2024-05-01","poster_path":"/p.jpg"},{"id":12,"title":"M12","release_date":"2025-05-01","poster_path":"/p.jpg"}]}`))
		case "/3/tv/21/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Pilot","runtime":42}]}`))
		case "/3/tv/21":
			_, _ = w.Write([]byte(`{"id":21,"name":"S21","seasons":[{"season_number":1,"episode_count":8,"air_date":"2022-01-01"}],"created_by":[{"id":6,"name":"Cee"}]}`))
		case "/3/person/5/movie_credits":
			_, _ = w.Write([]byte(`{"crew":[{"id":11,"title":"M11","job":"Director","popularity":50},{"id":12,"title":"M12","job":"Director","popularity":9},
				{"id":14,"title":"M14","job":"Director","popularity":7},{"id":15,"title":"M15","job":"Director","popularity":8},{"id":16,"title":"M16","job":"Director","popularity":1}]}`))
		case "/3/person/6/tv_credits":
			_, _ = w.Write([]byte(`{"crew":[{"id":21,"name":"S21","job":"Creator"},{"id":22,"name":"S22","job":"Creator"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *discoverTMDB) provider() *metadata.TMDB {
	return &metadata.TMDB{BaseURL: f.srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

func discoverRig(t *testing.T, p metadata.Provider, off ...parts.Part) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`); err != nil {
			return err
		}
		title := mediafiles.Title{Kind: "movie", Name: "M11", SortName: "m11", SourceIDs: map[string]string{"tmdb": "11"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		show := mediafiles.Title{Kind: "show", Name: "S22", SortName: "s22", SourceIDs: map[string]string{"tmdb": "22", "tvdb": "9"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "running"}
		if _, err := database.Insert(tx, "titles", &show, domain.System); err != nil {
			return err
		}
		never := mediafiles.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "13"}, Name: "M13", Reason: "test", CreatedAt: time.Now()}
		if _, err := database.Insert(tx, "never_add", &never, domain.System); err != nil {
			return err
		}
		ask := domain.Ask{Kind: "movie", TMDBID: "12", Name: "M12", State: domain.AskAsked, AskedBy: 1, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "12"}}
		if _, err := database.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 1, ?, 1)", ask.ID, database.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var pc *parts.Control
	if len(off) > 0 {
		pc = &parts.Control{Store: s, Reg: jobs.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
		if err := pc.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, part := range off {
			if err := pc.Set(context.Background(), part, false, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: p, Parts: pc, Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

type discoverAnswer struct {
	Row   string `json:"row"`
	Title string `json:"title"`
	Items []struct {
		TMDBID   string    `json:"tmdb_id"`
		Kind     string    `json:"kind"`
		Name     string    `json:"name"`
		Genres   []string  `json:"genres"`
		InLib    bool      `json:"in_library"`
		TitleID  int64     `json:"title_id"`
		Ask      *askBrief `json:"ask"`
		NeverAdd *bool     `json:"never_add"`
	} `json:"items"`
}

func getDiscover(t *testing.T, url string, want int) (discoverAnswer, http.Header) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var a discoverAnswer
	_ = json.NewDecoder(resp.Body).Decode(&a)
	if resp.StatusCode != want {
		t.Fatalf("%s: %d, want %d", url, resp.StatusCode, want)
	}
	return a, resp.Header
}

// route: GET /api/v1/discover
func TestDiscoverRowsJoinLibraryAndAsks(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider())
	a, h := getDiscover(t, srv.URL+"/api/v1/discover?row=trending&kind=movie", 200)
	if a.Row != "trending" || a.Title == "" || len(a.Items) != 20 || h.Get("Cache-Control") != "private, max-age=300" {
		t.Fatalf("answer %+v %v", a, h)
	}
	byID := map[string]int{}
	for i, it := range a.Items {
		byID[it.TMDBID] = i
	}
	if _, ok := byID["13"]; ok {
		t.Fatal("never-add item listed")
	}
	m11, m12 := a.Items[byID["11"]], a.Items[byID["12"]]
	if !m11.InLib || m11.TitleID != 1 || m11.Ask != nil || m11.Genres[0] != "Drama" {
		t.Fatalf("m11 %+v", m11)
	}
	if m12.InLib || m12.Ask == nil || m12.Ask.State != "asked" || !m12.Ask.Mine || m12.Ask.People != 1 || m12.Ask.StateLabel == "" {
		t.Fatalf("m12 %+v", m12)
	}
	both, _ := getDiscover(t, srv.URL+"/api/v1/discover?row=trending", 200)
	kinds := map[string]bool{}
	for _, it := range both.Items {
		kinds[it.Kind] = true
	}
	if len(both.Items) != 20 || !kinds["movie"] || !kinds["show"] {
		t.Fatalf("both kinds %+v", both)
	}
	sim, _ := getDiscover(t, srv.URL+"/api/v1/discover?row=similar&id=1", 200)
	if len(sim.Items) != 2 || sim.Items[0].TMDBID != "41" || sim.Items[0].Kind != "movie" {
		t.Fatalf("similar %+v", sim)
	}
	g, _ := getDiscover(t, srv.URL+"/api/v1/discover?row=genre&g=Drama&kind=show", 200)
	if len(g.Items) != 1 || g.Items[0].Kind != "show" {
		t.Fatalf("genre %+v", g)
	}
	// Cached: a second ask reaches TMDB no more, but joins stay fresh.
	before := f.hits.Load()
	getDiscover(t, srv.URL+"/api/v1/discover?row=trending&kind=movie", 200)
	if f.hits.Load() != before {
		t.Fatal("cached row went back to TMDB")
	}
	for _, bad := range []string{"", "?row=nope", "?row=similar", "?row=similar&id=x", "?row=genre", "?row=trending&kind=artist"} {
		getDiscover(t, srv.URL+"/api/v1/discover"+bad, 400)
	}
	getDiscover(t, srv.URL+"/api/v1/discover?row=similar&id=999", 404)
}

func TestDiscoverNotSetUpRateLimitedAndPerPersonLimit(t *testing.T) {
	t.Parallel()
	srv, _ := discoverRig(t, metadata.Lab())
	getDiscover(t, srv.URL+"/api/v1/discover?row=trending", 501)

	f := newDiscoverTMDB(t)
	srv2, _ := discoverRig(t, f.provider())
	getDiscover(t, srv2.URL+"/api/v1/discover?row=popular&kind=movie", 200)
	f.limited.Store(true)
	if a, _ := getDiscover(t, srv2.URL+"/api/v1/discover?row=popular&kind=movie", 200); len(a.Items) != 1 {
		t.Fatalf("cached answer while limited %+v", a)
	}
	_, h := getDiscover(t, srv2.URL+"/api/v1/discover?row=upcoming&kind=movie", 503)
	if h.Get("Retry-After") == "" {
		t.Fatal("503 without Retry-After")
	}
	f.limited.Store(false)
	limited := false
	for i := 0; i <= discover.PerMinute; i++ {
		resp, err := http.Get(srv2.URL + "/api/v1/discover?row=popular&kind=movie")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == 429 {
			limited = resp.Header.Get("Retry-After") != ""
			break
		}
	}
	if !limited {
		t.Fatal("no 429 over the minute limit")
	}
}

func TestDiscoverFollowsParts(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider(), parts.Shows)
	a, _ := getDiscover(t, srv.URL+"/api/v1/discover?row=trending&kind=show", 200)
	if len(a.Items) != 0 {
		t.Fatalf("shows off %+v", a)
	}
	a, _ = getDiscover(t, srv.URL+"/api/v1/discover?row=trending", 200)
	for _, it := range a.Items {
		if it.Kind == "show" {
			t.Fatalf("show listed with shows off %+v", a)
		}
	}
}

// route: GET /api/v1/metadata/{kind}/{tmdb_id}
func TestMetadataDetails(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider())
	b := string(do(t, "GET", srv.URL+"/api/v1/metadata/movie/11", "", 200))
	for _, want := range []string{`"tmdb_id":"11"`, `"kind":"movie"`, `"runtime_minutes":100`, `"in_library":true`, `"title_id":1`,
		`"character":"Hero"`, `"ask":null`, `"collection":{"id":"90","name":"The Set","parts":[{"tmdb_id":"11"`, `"streaming":[]`} {
		if !strings.Contains(b, want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
	var d struct {
		Similar []struct {
			TMDBID string    `json:"tmdb_id"`
			Ask    *askBrief `json:"ask"`
		} `json:"similar"`
	}
	_ = json.Unmarshal([]byte(b), &d)
	if len(d.Similar) != 2 || d.Similar[0].TMDBID != "12" || d.Similar[0].Ask == nil || d.Similar[1].TMDBID != "41" {
		t.Fatalf("similar %s", b)
	}
	before := f.details.Load()
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/11", "", 200)
	if f.details.Load() != before {
		t.Fatal("details not cached")
	}
	s := string(do(t, "GET", srv.URL+"/api/v1/metadata/show/21", "", 200))
	if !strings.Contains(s, `"episode_count":8`) || !strings.Contains(s, `"role":"creator"`) || !strings.Contains(s, `"in_library":false`) {
		t.Fatalf("show %s", s)
	}
	if !strings.Contains(s, `"streaming":[]`) {
		t.Fatalf("streaming US %s", s)
	}
	if de := string(do(t, "GET", srv.URL+"/api/v1/metadata/show/21?region=de", "", 200)); !strings.Contains(de, `"streaming":[{"id":8,"name":"Netflix"`) {
		t.Fatalf("streaming DE %s", de)
	}
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/404", "", 404)
	do(t, "GET", srv.URL+"/api/v1/metadata/artist/1", "", 400)
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/abc", "", 400)

	srv2, _ := discoverRig(t, metadata.Lab())
	do(t, "GET", srv2.URL+"/api/v1/metadata/movie/11", "", 501)
}

func TestMetadataDetailsRowsCarryCardLogos(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, s := discoverRig(t, f.provider())
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO card_art (kind, tmdb_id, lang, art, logo, has_title, fetched_at) VALUES ('movie', '12', 'en', '', 'https://img/logo12.png', 0, ?)`, database.FormatTime(time.Now()))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/metadata/movie/11", "", 200))
	if !strings.Contains(b, `"card_logo":"https://img/logo12.png"`) {
		t.Fatalf("similar logo missing %s", b)
	}
}

func TestQualityWord(t *testing.T) {
	t.Parallel()
	for res, want := range map[int64]string{0: "", 480: "", 720: "HD", 1080: "HD", 2160: "4K", 4320: "4K"} {
		if got := discover.QualityWord(res); got != want {
			t.Errorf("%d: %q, want %q", res, got, want)
		}
	}
}

// route: GET /api/v1/metadata/{kind}/{tmdb_id}/more-from
func TestMetadataMoreFrom(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider())
	var m struct {
		Person string `json:"person"`
		Role   string `json:"role"`
		Items  []struct {
			TMDBID    string `json:"tmdb_id"`
			InLibrary bool   `json:"in_library"`
		} `json:"items"`
	}
	b := do(t, "GET", srv.URL+"/api/v1/metadata/movie/11/more-from", "", 200)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Person != "Dee" || m.Role != "director" || len(m.Items) != 4 || m.Items[0].TMDBID != "12" || m.Items[1].TMDBID != "15" || m.Items[3].TMDBID != "16" {
		t.Fatalf("more from %s", b)
	}
	if d := string(do(t, "GET", srv.URL+"/api/v1/metadata/movie/11", "", 200)); !strings.Contains(d, `"moods":["Drama"]`) {
		t.Fatalf("moods %s", d)
	}
	before := f.hits.Load()
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/11/more-from", "", 200)
	if f.hits.Load() != before {
		t.Fatal("more from not cached")
	}
	// Under four titles: no row.
	s := string(do(t, "GET", srv.URL+"/api/v1/metadata/show/21/more-from", "", 200))
	if !strings.Contains(s, `"role":"creator"`) || !strings.Contains(s, `"items":[]`) {
		t.Fatalf("show %s", s)
	}
	do(t, "GET", srv.URL+"/api/v1/metadata/artist/1/more-from", "", 400)
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/abc/more-from", "", 400)
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/404/more-from", "", 404)
	srv2, _ := discoverRig(t, metadata.Lab())
	do(t, "GET", srv2.URL+"/api/v1/metadata/movie/11/more-from", "", 501)
}

// route: GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}
func TestMetadataSeason(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider())
	b := string(do(t, "GET", srv.URL+"/api/v1/metadata/show/21/seasons/1", "", 200))
	if !strings.Contains(b, `"season":1`) || !strings.Contains(b, `"name":"Pilot"`) || !strings.Contains(b, `"runtime":42`) {
		t.Fatalf("season %s", b)
	}
	before := f.hits.Load()
	do(t, "GET", srv.URL+"/api/v1/metadata/show/21/seasons/1", "", 200)
	if f.hits.Load() != before {
		t.Fatal("season not cached")
	}
	do(t, "GET", srv.URL+"/api/v1/metadata/show/21/seasons/9", "", 404)
	do(t, "GET", srv.URL+"/api/v1/metadata/show/abc/seasons/1", "", 400)
	do(t, "GET", srv.URL+"/api/v1/metadata/show/21/seasons/x", "", 400)
	srv2, _ := discoverRig(t, f.provider(), parts.Shows)
	do(t, "GET", srv2.URL+"/api/v1/metadata/show/21/seasons/1", "", 409)
	srv3, _ := discoverRig(t, metadata.Lab())
	do(t, "GET", srv3.URL+"/api/v1/metadata/show/21/seasons/1", "", 501)
}

// route: GET /api/v1/watch-services
func TestDiscoverServicesRowAndServiceList(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider())
	a, _ := getDiscover(t, srv.URL+"/api/v1/discover?row=services&services=8%7C337&region=us&kind=movie", 200)
	if a.Row != "services" || a.Title != "On your services" || len(a.Items) != 1 || a.Items[0].TMDBID != "31" {
		t.Fatalf("services row %+v", a)
	}
	for _, bad := range []string{"?row=services", "?row=services&services=8", "?row=services&services=8,9&region=US", "?row=services&services=0&region=US", "?row=services&services=8&region=USA"} {
		getDiscover(t, srv.URL+"/api/v1/discover"+bad, 400)
	}
	resp, err := http.Get(srv.URL + "/api/v1/watch-services?region=US")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Region   string                  `json:"region"`
		Services []metadata.WatchService `json:"services"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || resp.StatusCode != 200 {
		t.Fatalf("list %d %v", resp.StatusCode, err)
	}
	if got.Region != "US" || len(got.Services) != 3 || got.Services[0].Name != "Hulu" || got.Services[1].Name != "Netflix" || got.Services[1].Priority != 1 ||
		!strings.HasSuffix(got.Services[1].Logo, "/w92/n.jpg") || got.Services[2].ID != 337 {
		t.Fatalf("list %+v", got)
	}
	r2, _ := http.Get(srv.URL + "/api/v1/watch-services?region=x")
	r2.Body.Close()
	if r2.StatusCode != 400 {
		t.Fatalf("bad region %d", r2.StatusCode)
	}
}

func TestDetailsWithMissedCollectionAreKeptBriefly(t *testing.T) {
	t.Parallel()
	if got := discover.DetailsTTL(discover.Entry{Details: &metadata.Details{CollectionMissed: true}}); got != discover.CollectionRetry {
		t.Fatalf("missed collection kept %v", got)
	}
	if got := discover.DetailsTTL(discover.Entry{Details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "k"}}}}); got != discover.TTL["details"] {
		t.Fatalf("details kept %v", got)
	}
}

// Details cached before cast ids were kept are read again; ones with ids keep their TTL.
func TestDetailsTTLRefreshesPeopleWithoutIDs(t *testing.T) {
	t.Parallel()
	old := discover.Entry{Details: &metadata.Details{People: []metadata.DetailPerson{{Name: "Ann", Role: "actor"}}}}
	if got := discover.DetailsTTL(old); got != 0 {
		t.Fatalf("no ids: ttl %v, want 0", got)
	}
	fresh := discover.Entry{Details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "k"}}, People: []metadata.DetailPerson{{Name: "Ann", Role: "actor", ID: "21"}}}}
	if got := discover.DetailsTTL(fresh); got != discover.TTL["details"] {
		t.Fatalf("with ids: ttl %v", got)
	}
	if got := discover.DetailsTTL(discover.Entry{Details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "k"}}}}); got != discover.TTL["details"] {
		t.Fatalf("no people: ttl %v", got)
	}
}
