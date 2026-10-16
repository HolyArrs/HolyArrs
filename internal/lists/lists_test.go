package lists

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Test-only stand-ins; the fakes refuse anything else.
const (
	fakeTMDBToken   = "tmdb-token-for-tests"
	fakeTraktClient = "trakt-client-for-tests"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func day(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

var movies = []metadata.Movie{
	{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002", "imdb": "tt9000020"}, CinemaDate: day("2023-05-05"), DigitalDate: day("2023-07-01")},
	{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003", "imdb": "tt9000030"}, CinemaDate: day("2022-09-09")},
	{Name: "Glass Meridian", Year: 2021, IDs: map[string]string{"tmdb": "900004", "imdb": "tt9000040"}, CinemaDate: day("2021-03-03")},
	{Name: "Salt Choir", Year: 2020, IDs: map[string]string{"tmdb": "900005", "imdb": "tt9000050"}, CinemaDate: day("2020-01-01")},
}

// fakeTMDB serves list 42 over two pages (one show mixed in) and refuses a
// missing or wrong token. It never reaches the real TMDB.
func fakeTMDB(t *testing.T, hits *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+fakeTMDBToken {
			w.WriteHeader(401)
			return
		}
		if strings.Contains(r.URL.RawQuery, fakeTMDBToken) {
			t.Error("token in the URL")
		}
		if r.URL.Path != "/list/42" {
			w.WriteHeader(404)
			return
		}
		page := r.URL.Query().Get("page")
		items := []map[string]any{{"id": 900002, "media_type": "movie", "title": "Paper Harbour"}, {"id": 77, "media_type": "tv", "name": "A Show"}}
		if page == "2" {
			items = []map[string]any{{"id": 900003, "media_type": "movie", "title": "Quiet Orchard"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "name": "Lab list", "page": page, "total_pages": 2, "items": items})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeTrakt serves luis/favourites and luis's watchlist.
func fakeTrakt(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("trakt-api-key") != fakeTraktClient || r.Header.Get("trakt-api-version") != "2" {
			w.WriteHeader(403)
			return
		}
		var out []map[string]any
		switch r.URL.Path {
		case "/users/luis/lists/favourites/items/movie":
			out = []map[string]any{
				{"type": "movie", "movie": map[string]any{"title": "Glass Meridian", "year": 2021, "ids": map[string]any{"tmdb": 900004, "imdb": "tt9000040"}}},
				{"type": "movie", "movie": map[string]any{"title": "Salt Choir", "year": 2020, "ids": map[string]any{"imdb": "tt9000050"}}},
			}
		case "/users/luis/watchlist/movies":
			out = []map[string]any{{"type": "movie", "movie": map[string]any{"title": "Quiet Orchard", "ids": map[string]any{"tmdb": 900003}}}}
		default:
			w.WriteHeader(404)
			return
		}
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type env struct {
	s     *store.Store
	q     *jobs.Queue
	svc   *Service
	lf    domain.LibraryFolder
	hits  *atomic.Int32
	clock *time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	md := &metadata.Static{Movies: movies}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	clock := now
	lib := &library.Service{Store: s, Queue: q, Metadata: md, Now: func() time.Time { return clock }}
	lib.Register(q)
	q.Handle(library.KindSearchSlot, func(context.Context, *jobs.Run) error { return nil })
	hits := &atomic.Int32{}
	svc := &Service{Store: s, Queue: q, Library: lib, Metadata: md,
		TMDBBase: fakeTMDB(t, hits).URL, TraktBase: fakeTrakt(t).URL,
		TMDBToken:   func(context.Context) (string, error) { return fakeTMDBToken, nil },
		TraktClient: func(context.Context) (string, error) { return fakeTraktClient, nil },
		Now:         func() time.Time { return clock }}
	svc.Register(q)
	lf, err := library.AddFolder(context.Background(), s, library.FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return &env{s: s, q: q, svc: svc, lf: lf, hits: hits, clock: &clock}
}

func titles(t *testing.T, s *store.Store) map[string]domain.Title {
	t.Helper()
	ts, err := store.Select[domain.Title](context.Background(), s.Read(), "titles", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.Title{}
	for _, x := range ts {
		out[x.Name] = x
	}
	return out
}

func TestListInputChecks(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	bad := []Input{
		{Name: "", Kind: KindTMDB, Ref: "42"},
		{Name: "x", Kind: "imdb", Ref: "42"},
		{Name: "x", Kind: KindTMDB, Ref: "forty-two"},
		{Name: "x", Kind: KindTrakt, Ref: "no-slash"},
		{Name: "x", Kind: KindTMDB, Ref: "42", SyncEveryHours: 1},
		{Name: "x", Kind: KindTMDB, Ref: "42", LibraryFolderID: 999},
		{Name: "x", Kind: KindTMDB, Ref: "42", PickRulesID: 999},
	}
	for _, in := range bad {
		if _, err := Add(ctx, e.s, in, domain.System); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	l, err := Add(ctx, e.s, Input{Name: "Lab list", Kind: KindTMDB, Ref: "https://www.themoviedb.org/list/42-lab-list"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if l.Ref != "42" || l.SyncEveryHours != 12 || !l.Enabled || l.LibraryFolderID != e.lf.ID {
		t.Fatalf("defaults %+v", l)
	}
	if _, err := Add(ctx, e.s, Input{Name: "lab list", Kind: KindTMDB, Ref: "43"}, domain.System); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("same name: %v", err)
	}
	off := false
	l, err = Edit(ctx, e.s, l.ID, Input{Name: "Lab list", Kind: KindTMDB, Ref: "42", Enabled: &off, SyncEveryHours: 24}, domain.System)
	if err != nil || l.Enabled || l.SyncEveryHours != 24 {
		t.Fatalf("edit %+v %v", l, err)
	}
	if err := Remove(ctx, e.s, l.ID); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, e.s, l.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
}

func TestTMDBListAddsMoviesAndSkipsNeverAdd(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := library.AddNeverAdd(ctx, e.s, domain.NeverAdd{SourceIDs: map[string]string{"tmdb": "900003"}, Name: "Quiet Orchard"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l, err := Add(ctx, e.s, Input{Name: "Lab list", Kind: KindTMDB, Ref: "42"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 2 || res.Added != 1 || res.NeverAdd != 1 || res.Already != 0 {
		t.Fatalf("result %+v", res)
	}
	ts := titles(t, e.s)
	if ts["Paper Harbour"].WhyAdded != "list" || len(ts) != 1 {
		t.Fatalf("titles %+v", ts)
	}
	got, _ := Get(ctx, e.s.Read(), l.ID)
	if got.LastSyncedAt == nil || got.LastError != "" || !strings.Contains(got.LastResult, "1 added") {
		t.Fatalf("list after sync %+v", got)
	}
	// Again: nothing new, the one already added counts as already there.
	res, _ = e.svc.Sync(ctx, l.ID, domain.System)
	if res.Added != 0 || res.Already != 1 || res.NeverAdd != 1 {
		t.Fatalf("second %+v", res)
	}
	evs, _ := store.Select[domain.Event](ctx, e.s.ReadEvents(), "events", "kind = ?", domain.EventListSynced)
	if len(evs) != 2 {
		t.Fatalf("events %d", len(evs))
	}
}

func TestTraktListAndWatchlist(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	l, err := Add(ctx, e.s, Input{Name: "Favourites", Kind: KindTrakt, Ref: "luis/favourites", SearchOnAdd: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil || res.Added != 2 {
		t.Fatalf("result %+v %v", res, err) // Salt Choir has only an IMDb id
	}
	w, _ := Add(ctx, e.s, Input{Name: "Watchlist", Kind: KindTrakt, Ref: "https://trakt.tv/users/luis/watchlist"}, domain.System)
	if w.Ref != "luis/watchlist" {
		t.Fatalf("ref %q", w.Ref)
	}
	if res, err := e.svc.Sync(ctx, w.ID, domain.System); err != nil || res.Added != 1 {
		t.Fatalf("watchlist %+v %v", res, err)
	}
	if len(titles(t, e.s)) != 3 {
		t.Fatal("expected three movies")
	}
}

func TestFailedFetchIsRecorded(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.svc.TMDBToken = func(context.Context) (string, error) { return "wrong", nil }
	l, _ := Add(ctx, e.s, Input{Name: "Lab list", Kind: KindTMDB, Ref: "42"}, domain.System)
	if _, err := e.svc.Sync(ctx, l.ID, domain.System); err == nil {
		t.Fatal("no error from a refused token")
	}
	got, _ := Get(ctx, e.s.Read(), l.ID)
	if got.LastError == "" || strings.Contains(got.LastError, fakeTMDBToken) {
		t.Fatalf("error %q", got.LastError)
	}
	e.svc.TraktClient = func(context.Context) (string, error) { return "", nil }
	tl, _ := Add(ctx, e.s, Input{Name: "Trakt", Kind: KindTrakt, Ref: "luis/favourites"}, domain.System)
	if _, err := e.svc.Sync(ctx, tl.ID, domain.System); !errors.Is(err, ErrNoTraktKey) {
		t.Fatalf("no key: %v", err)
	}
}

func TestScheduledSyncOnlyDueEnabledLists(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.q.Stop)
	off := false
	_, _ = Add(ctx, e.s, Input{Name: "Lab list", Kind: KindTMDB, Ref: "42"}, domain.System)
	_, _ = Add(ctx, e.s, Input{Name: "Off", Kind: KindTMDB, Ref: "42", Enabled: &off}, domain.System)
	var sched int
	_ = e.s.Read().QueryRow("SELECT count(*) FROM schedules WHERE kind = ? AND enabled = 1", KindSync).Scan(&sched)
	if sched != 1 {
		t.Fatal("no schedule row for list syncs")
	}
	run := func() {
		t.Helper()
		before := e.hits.Load()
		id, _, err := e.q.Enqueue(ctx, KindSync, nil, map[string]any{"scheduled": true}, "", domain.System)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var j domain.Job
			_ = store.Get(ctx, e.s.Read(), "jobs", id, &j)
			if j.State == "done" || j.State == "failed" {
				if j.State == "failed" {
					t.Fatalf("job failed: %+v", j.Error)
				}
				t.Logf("hits %d", e.hits.Load()-before)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("sync job never finished")
	}
	run()
	if e.hits.Load() != 2 { // two pages of the one enabled list
		t.Fatalf("hits %d", e.hits.Load())
	}
	run() // not due yet
	if e.hits.Load() != 2 {
		t.Fatalf("synced again too soon: %d", e.hits.Load())
	}
	*e.clock = now.Add(13 * time.Hour)
	run()
	if e.hits.Load() != 4 {
		t.Fatalf("not synced when due: %d", e.hits.Load())
	}
}
