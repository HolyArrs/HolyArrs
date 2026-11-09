package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Keys the fake servers accept. They refuse any other key.
const (
	fakeTMDBToken   = "tmdb-token-for-tests"
	fakeTraktClient = "trakt-client-for-tests"
)

// now is the fixed clock the tests start from.
var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// day parses a YYYY-MM-DD date for the lab titles.
func day(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

// movies are the made-up titles the metadata stand-in knows.
var movies = []metadata.Movie{
	{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002", "imdb": "tt9000020"}, CinemaDate: day("2023-05-05"), DigitalDate: day("2023-07-01")},
	{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003", "imdb": "tt9000030"}, CinemaDate: day("2022-09-09")},
	{Name: "Glass Meridian", Year: 2021, IDs: map[string]string{"tmdb": "900004", "imdb": "tt9000040"}, CinemaDate: day("2021-03-03")},
	{Name: "Salt Choir", Year: 2020, IDs: map[string]string{"tmdb": "900005", "imdb": "tt9000050"}, CinemaDate: day("2020-01-01")},
}

// fakeTMDB serves list 42 over two pages with one show mixed in, and refuses
// a missing or wrong token. It never reaches the real TMDB.
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

// fakeTrakt serves the list kim/favourites and the watchlist of kim.
func fakeTrakt(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("trakt-api-key") != fakeTraktClient || r.Header.Get("trakt-api-version") != "2" {
			w.WriteHeader(403)
			return
		}
		var out []map[string]any
		switch r.URL.Path {
		case "/users/kim/lists/favourites/items/movie":
			out = []map[string]any{
				{"type": "movie", "movie": map[string]any{"title": "Glass Meridian", "year": 2021, "ids": map[string]any{"tmdb": 900004, "imdb": "tt9000040"}}},
				{"type": "movie", "movie": map[string]any{"title": "Salt Choir", "year": 2020, "ids": map[string]any{"imdb": "tt9000050"}}},
			}
		case "/users/kim/watchlist/movies":
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
	s     *database.Store
	q     *jobs.Queue
	svc   *OutsideLists
	lf    mediafiles.LibraryFolder
	hits  *atomic.Int32
	clock *time.Time
}

// setup opens a store and builds a OutsideLists wired to the fake servers.
func setup(t *testing.T) *env {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
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
	svc := &OutsideLists{Store: s, Queue: q, Library: lib, Metadata: md,
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

// titles returns the Library titles by name.
func titles(t *testing.T, s *database.Store) map[string]mediafiles.Title {
	t.Helper()
	ts, err := database.Select[mediafiles.Title](context.Background(), s.Read(), "titles", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]mediafiles.Title{}
	for _, x := range ts {
		out[x.Name] = x
	}
	return out
}

func TestAdd_InputChecks(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	bad := []OutsideListInput{
		{Name: "", Kind: ListTMDB, Ref: "42"},
		{Name: "x", Kind: "imdb", Ref: "42"},
		{Name: "x", Kind: ListTMDB, Ref: "forty-two"},
		{Name: "x", Kind: ListTrakt, Ref: "no-slash"},
		{Name: "x", Kind: ListTMDB, Ref: "42", SyncEveryHours: 1},
		{Name: "x", Kind: ListTMDB, Ref: "42", LibraryFolderID: 999},
		{Name: "x", Kind: ListTMDB, Ref: "42", PickRulesID: 999},
	}
	for _, in := range bad {
		if _, err := AddOutsideList(ctx, e.s, in, domain.System); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Lab list", Kind: ListTMDB, Ref: "https://www.themoviedb.org/list/42-lab-list"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if l.Ref != "42" || l.SyncEveryHours != 12 || !l.Enabled || l.LibraryFolderID != e.lf.ID {
		t.Fatalf("defaults %+v", l)
	}
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "lab list", Kind: ListTMDB, Ref: "43"}, domain.System); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("same name: %v", err)
	}
	off := false
	l, err = EditOutsideList(ctx, e.s, l.ID, OutsideListInput{Name: "Lab list", Kind: ListTMDB, Ref: "42", Enabled: &off, SyncEveryHours: 24}, domain.System)
	if err != nil || l.Enabled || l.SyncEveryHours != 24 {
		t.Fatalf("edit %+v %v", l, err)
	}
	if err := RemoveOutsideList(ctx, e.s, l.ID); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOutsideList(ctx, e.s, l.ID); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	// A step given to AddOutsideList runs in the same write, so when it fails no list
	// is kept.
	var got int64
	l, err = AddOutsideList(ctx, e.s, OutsideListInput{Name: "Hooked", Kind: ListTMDB, Ref: "44"}, domain.System, func(tx *sql.Tx, id int64) error {
		got = id
		return nil
	})
	if err != nil || got == 0 {
		t.Fatalf("then step: id %d, %v", got, err)
	}
	failed := errors.New("mark failed")
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Dropped", Kind: ListTMDB, Ref: "45"}, domain.System, func(*sql.Tx, int64) error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("failing step: %v", err)
	}
	if all, _ := AllOutsideLists(ctx, e.s.Read()); len(all) != 1 || all[0].Name != "Hooked" {
		t.Fatalf("lists %+v", all)
	}
}

func TestSync_TMDBListAddsMoviesAndSkipsNeverAdd(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := library.AddNeverAdd(ctx, e.s, mediafiles.NeverAdd{SourceIDs: map[string]string{"tmdb": "900003"}, Name: "Quiet Orchard"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Lab list", Kind: ListTMDB, Ref: "42"}, domain.System)
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
	got, _ := GetOutsideList(ctx, e.s.Read(), l.ID)
	if got.LastSyncedAt == nil || got.LastError != "" || !strings.Contains(got.LastResult, "1 added") {
		t.Fatalf("list after sync %+v", got)
	}
	// Second run: nothing new, the added title counts as already there.
	res, _ = e.svc.Sync(ctx, l.ID, domain.System)
	if res.Added != 0 || res.Already != 1 || res.NeverAdd != 1 {
		t.Fatalf("second %+v", res)
	}
	evs, _ := database.Select[domain.Event](ctx, e.s.ReadEvents(), "events", "kind = ?", domain.EventListSynced)
	if len(evs) != 2 {
		t.Fatalf("events %d", len(evs))
	}
}

func TestSync_TraktListAndWatchlist(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Favourites", Kind: ListTrakt, Ref: "kim/favourites", SearchOnAdd: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil || res.Added != 2 {
		t.Fatalf("result %+v %v", res, err) // Salt Choir has only an IMDb id
	}
	w, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Watchlist", Kind: ListTrakt, Ref: "https://trakt.tv/users/kim/watchlist"}, domain.System)
	if w.Ref != "kim/watchlist" {
		t.Fatalf("ref %q", w.Ref)
	}
	if res, err := e.svc.Sync(ctx, w.ID, domain.System); err != nil || res.Added != 1 {
		t.Fatalf("watchlist %+v %v", res, err)
	}
	if len(titles(t, e.s)) != 3 {
		t.Fatal("expected three movies")
	}
}

func TestSync_FailedFetchIsRecorded(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.svc.TMDBToken = func(context.Context) (string, error) { return "wrong", nil }
	l, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Lab list", Kind: ListTMDB, Ref: "42"}, domain.System)
	if _, err := e.svc.Sync(ctx, l.ID, domain.System); err == nil {
		t.Fatal("no error from a refused token")
	}
	got, _ := GetOutsideList(ctx, e.s.Read(), l.ID)
	if got.LastError == "" || strings.Contains(got.LastError, fakeTMDBToken) {
		t.Fatalf("error %q", got.LastError)
	}
	e.svc.TraktClient = func(context.Context) (string, error) { return "", nil }
	tl, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Trakt", Kind: ListTrakt, Ref: "kim/favourites"}, domain.System)
	if _, err := e.svc.Sync(ctx, tl.ID, domain.System); !errors.Is(err, ErrNoTraktKey) {
		t.Fatalf("no key: %v", err)
	}
}

func TestHandle_ScheduleReadsOnlyDueEnabledLists(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.q.Stop)
	off := false
	_, _ = AddOutsideList(ctx, e.s, OutsideListInput{Name: "Lab list", Kind: ListTMDB, Ref: "42"}, domain.System)
	_, _ = AddOutsideList(ctx, e.s, OutsideListInput{Name: "Off", Kind: ListTMDB, Ref: "42", Enabled: &off}, domain.System)
	var sched int
	_ = e.s.Read().QueryRow("SELECT count(*) FROM schedules WHERE kind = ? AND enabled = 1", OutsideListSync).Scan(&sched)
	if sched != 1 {
		t.Fatal("no schedule row for list syncs")
	}
	run := func() {
		t.Helper()
		before := e.hits.Load()
		id, _, err := e.q.Enqueue(ctx, OutsideListSync, nil, map[string]any{"scheduled": true}, "", domain.System)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var j domain.Job
			_ = database.Get(ctx, e.s.Read(), "jobs", id, &j)
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
