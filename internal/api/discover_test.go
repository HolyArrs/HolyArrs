package api

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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

type discoverTMDB struct {
	srv     *httptest.Server
	hits    atomic.Int32
	limited atomic.Bool
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
			_, _ = w.Write([]byte(`{"id":11,"title":"M11","release_date":"2024-05-01","runtime":100,"genres":[{"name":"Drama"}],
				"credits":{"cast":[{"name":"Ann","character":"Hero"}]},"belongs_to_collection":{"id":90,"name":"Set"},"recommendations":` + list(12, 13, 41) + `}`))
		case "/3/collection/90":
			_, _ = w.Write([]byte(`{"name":"The Set","parts":[{"id":11,"title":"M11","release_date":"2024-05-01","poster_path":"/p.jpg"},{"id":12,"title":"M12","release_date":"2025-05-01","poster_path":"/p.jpg"}]}`))
		case "/3/tv/21/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Pilot","runtime":42}]}`))
		case "/3/tv/21":
			_, _ = w.Write([]byte(`{"id":21,"name":"S21","watch/providers":{"results":{"DE":{"flatrate":[{"provider_id":8,"provider_name":"Netflix"}]}}},"seasons":[{"season_number":1,"episode_count":8,"air_date":"2022-01-01"}],"created_by":[{"name":"Cee"}]}`))
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

func discoverRig(t *testing.T, p metadata.Provider, off ...parts.Part) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`); err != nil {
			return err
		}
		title := domain.Title{Kind: "movie", Name: "M11", SortName: "m11", SourceIDs: map[string]string{"tmdb": "11"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := store.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		show := domain.Title{Kind: "show", Name: "S22", SortName: "s22", SourceIDs: map[string]string{"tmdb": "22", "tvdb": "9"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "running"}
		if _, err := store.Insert(tx, "titles", &show, domain.System); err != nil {
			return err
		}
		never := domain.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "13"}, Name: "M13", Reason: "test", CreatedAt: time.Now()}
		if _, err := store.Insert(tx, "never_add", &never, domain.System); err != nil {
			return err
		}
		ask := domain.Ask{Kind: "movie", TMDBID: "12", Name: "M12", State: domain.AskAsked, AskedBy: 1, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "12"}}
		if _, err := store.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 1, ?, 1)", ask.ID, store.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var pc *parts.Control
	if len(off) > 0 {
		pc = &parts.Control{Store: s, Reg: supervisor.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
		if err := pc.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, part := range off {
			if err := pc.Set(context.Background(), part, false, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: p, Parts: pc, Version: "test"}).Handler())
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
	for i := 0; i < 31; i++ {
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
		t.Fatal("no 429 after 30 requests a minute")
	}
}

func TestDiscoverFollowsParts(t *testing.T) {
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
	before := f.hits.Load()
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/11", "", 200)
	if f.hits.Load() != before {
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

// route: GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}
func TestMetadataSeason(t *testing.T) {
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
