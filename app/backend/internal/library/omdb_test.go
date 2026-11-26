package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeOMDb answers like OMDb: tt0000001 with every score, tt0000002 not
// found, and every request refused while limited is set.
func fakeOMDb(t *testing.T, asked *atomic.Int32, limited *atomic.Bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case limited.Load():
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"Response":"False","Error":"Request limit reached!"}`))
		case r.URL.Query().Get("i") == "tt0000001":
			_, _ = w.Write([]byte(`{"Title":"Dune: Part Two","imdbRating":"8.5","imdbVotes":"612,345","Metascore":"79",
				"Ratings":[{"Source":"Internet Movie Database","Value":"8.5/10"},{"Source":"Rotten Tomatoes","Value":"92%"},{"Source":"Metacritic","Value":"79/100"}],"Response":"True"}`))
		default:
			_, _ = w.Write([]byte(`{"Response":"False","Error":"Incorrect IMDb ID."}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func omdbRig(t *testing.T, base string, key *string) (*Service, *database.Store, *time.Time) {
	t.Helper()
	outbound.Reset()
	s := open(t)
	addFolder(t, s)
	clock := now
	svc := &Service{Store: s, Now: func() time.Time { return clock },
		OMDb: &metadata.OMDb{Base: base, Key: func(context.Context) (string, error) { return *key, nil }, HTTP: &http.Client{Timeout: 5 * time.Second}, Gap: time.Millisecond}}
	return svc, s, &clock
}

func addWithIMDb(t *testing.T, s *database.Store, name, tmdb, imdb string) int64 {
	t.Helper()
	ids := map[string]string{"tmdb": tmdb}
	if imdb != "" {
		ids["imdb"] = imdb
	}
	id, err := AddMovie(context.Background(), s, metadata.Movie{Name: name, Year: 2024, IDs: ids}, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func omdbTitle(t *testing.T, s *database.Store, id int64) mediafiles.Title {
	t.Helper()
	var ti mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", id, &ti); err != nil {
		t.Fatal(err)
	}
	return ti
}

func TestOMDbPassKeepsScoresAndAsksWeekly(t *testing.T) {
	var asked atomic.Int32
	var limited atomic.Bool
	srv := fakeOMDb(t, &asked, &limited)
	key := "omdb-fixture"
	svc, s, clock := omdbRig(t, srv.URL, &key)
	ctx := context.Background()
	found := addWithIMDb(t, s, "Dune: Part Two", "693134", "tt0000001")
	unknown := addWithIMDb(t, s, "The Lantern Keepers", "900001", "tt0000002")
	noID := addWithIMDb(t, s, "No IMDb", "900002", "")

	if err := svc.OMDbPass(ctx); err != nil || asked.Load() != 2 {
		t.Fatalf("first pass: %v, asked %d", err, asked.Load())
	}
	f := omdbTitle(t, s, found)
	if f.IMDbRating == nil || *f.IMDbRating != 8.5 || f.IMDbVotes != 612345 || f.RottenTomatoes == nil || *f.RottenTomatoes != 92 ||
		f.Metacritic == nil || *f.Metacritic != 79 || f.OMDbCheckedAt == nil {
		t.Fatalf("kept scores %+v", f)
	}
	u := omdbTitle(t, s, unknown)
	if u.IMDbRating != nil || u.RottenTomatoes != nil || u.OMDbCheckedAt == nil {
		t.Fatalf("a title OMDb does not know is marked asked, with no scores: %+v", u)
	}
	if n := omdbTitle(t, s, noID); n.OMDbCheckedAt != nil {
		t.Fatal("a title with no IMDb id is never asked")
	}
	sc, ok, err := mediafiles.OMDbScoresByTMDB(ctx, s.Read(), "movie", "693134")
	if err != nil || !ok || *sc.IMDbRating != 8.5 || *sc.Metacritic != 79 {
		t.Fatalf("by TMDB id: %+v %v %v", sc, ok, err)
	}
	if _, ok, _ := mediafiles.OMDbScoresByTMDB(ctx, s.Read(), "movie", "900001"); ok {
		t.Fatal("a title with no scores reads as none")
	}

	// Asked again only once the scores are a week old.
	*clock = clock.Add(6 * 24 * time.Hour)
	_ = svc.OMDbPass(ctx)
	if asked.Load() != 2 {
		t.Fatalf("asked again within the week: %d", asked.Load())
	}
	*clock = clock.Add(2 * 24 * time.Hour)
	svc.OMDb = &metadata.OMDb{Base: srv.URL, Key: svc.OMDb.Key, HTTP: svc.OMDb.HTTP, Gap: time.Millisecond} // no answers kept from before
	_ = svc.OMDbPass(ctx)
	if asked.Load() != 4 {
		t.Fatalf("not asked again after a week: %d", asked.Load())
	}
}

func TestOMDbPassRestsAfterTheLimit(t *testing.T) {
	var asked atomic.Int32
	var limited atomic.Bool
	limited.Store(true)
	srv := fakeOMDb(t, &asked, &limited)
	key := "omdb-fixture"
	svc, s, clock := omdbRig(t, srv.URL, &key)
	ctx := context.Background()
	id := addWithIMDb(t, s, "Dune: Part Two", "693134", "tt0000001")
	addWithIMDb(t, s, "The Lantern Keepers", "900001", "tt0000002")

	if err := svc.OMDbPass(ctx); err != nil {
		t.Fatalf("a refused key never fails the pass: %v", err)
	}
	if asked.Load() != 1 {
		t.Fatalf("the pass went on after the limit: %d", asked.Load())
	}
	if ti := omdbTitle(t, s, id); ti.OMDbCheckedAt != nil {
		t.Fatal("a refused request does not count as asked")
	}
	*clock = clock.Add(time.Hour)
	_ = svc.OMDbPass(ctx)
	if asked.Load() != 1 {
		t.Fatalf("asked while resting: %d", asked.Load())
	}
	// A new key ends the rest.
	limited.Store(false)
	key = "omdb-new-fixture"
	_ = svc.OMDbPass(ctx)
	if asked.Load() != 3 {
		t.Fatalf("a new key was not used: %d", asked.Load())
	}
	// The rest also ends on its own.
	limited.Store(true)
	*clock = clock.Add(8 * 24 * time.Hour)
	svc.OMDb = &metadata.OMDb{Base: srv.URL, Key: svc.OMDb.Key, HTTP: svc.OMDb.HTTP, Gap: time.Millisecond}
	_ = svc.OMDbPass(ctx)
	limited.Store(false)
	*clock = clock.Add(omdbBackOff + time.Minute)
	_ = svc.OMDbPass(ctx)
	if asked.Load() != 6 {
		t.Fatalf("did not try again after resting: %d", asked.Load())
	}
}

func TestOMDbPassWithoutAKeyAsksNothing(t *testing.T) {
	var asked atomic.Int32
	var limited atomic.Bool
	srv := fakeOMDb(t, &asked, &limited)
	key := ""
	svc, s, _ := omdbRig(t, srv.URL, &key)
	addWithIMDb(t, s, "Dune: Part Two", "693134", "tt0000001")
	if err := svc.OMDbPass(context.Background()); err != nil || asked.Load() != 0 {
		t.Fatalf("no key: %v, asked %d", err, asked.Load())
	}
	svc.OMDb = nil
	if err := svc.OMDbPass(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOMDbPassKeepsToItsShare(t *testing.T) {
	var asked atomic.Int32
	var limited atomic.Bool
	srv := fakeOMDb(t, &asked, &limited)
	key := "omdb-fixture"
	svc, s, _ := omdbRig(t, srv.URL, &key)
	for i := range omdbPerPass + 5 {
		addWithIMDb(t, s, "Movie "+string(rune('A'+i)), strconv.Itoa(900100+i), "tt"+strconv.Itoa(1000000+i))
	}
	_ = svc.OMDbPass(context.Background())
	if asked.Load() != omdbPerPass {
		t.Fatalf("one pass asked %d, want %d", asked.Load(), omdbPerPass)
	}
	svc.omdb.count = omdbPerDay - 2
	_ = svc.OMDbPass(context.Background())
	if asked.Load() != omdbPerPass+2 {
		t.Fatalf("the day's cap was passed: %d", asked.Load())
	}
}
