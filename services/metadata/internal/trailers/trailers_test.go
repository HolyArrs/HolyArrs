package trailers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	_ "modernc.org/sqlite"
)

type fakeTMDB struct {
	mu    sync.Mutex
	calls int
	paths []string
}

func (f *fakeTMDB) Fetch(_ context.Context, q model.Query) (model.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.paths = append(f.paths, q.Path+"?"+q.Params)
	switch q.Path {
	case "/movie/11/videos":
		return model.Result{Data: []byte(`{"results":[
			{"key":"clipKey0001","site":"YouTube","type":"Clip","official":true,"iso_639_1":"en"},
			{"key":"teaserKey01","site":"YouTube","type":"Teaser","official":true,"iso_639_1":"en"},
			{"key":"vimeoKey001","site":"Vimeo","type":"Trailer","official":true,"iso_639_1":"en"},
			{"key":"enTrailer01","name":"Official Trailer","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"},
			{"key":"deTrailer01","name":"Offizieller Trailer","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"de"}]}`), Complete: true}, nil
	case "/tv/12/videos":
		return model.Result{Data: []byte(`{"results":[]}`), Complete: true}, nil
	}
	return model.Result{}, &model.FetchError{Code: "upstream_status", Status: 404}
}

func open(t *testing.T, o Options) *Service {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "trailers.sqlite"), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPickRanksAndCaches(t *testing.T) {
	f := &fakeTMDB{}
	s := open(t, Options{Fetcher: f})
	ctx := context.Background()
	p, err := s.Pick(ctx, "movie", 11, "de", "tester")
	if err != nil || p.Key != "deTrailer01" || p.Language != "de" || p.Status != StatusUnchecked {
		t.Fatalf("de %+v %v", p, err)
	}
	if !strings.Contains(f.paths[0], "include_video_language=de%2Cen%2Cnull") {
		t.Fatalf("languages %v", f.paths)
	}
	p, err = s.Pick(ctx, "movie", 11, "fr", "tester")
	if err != nil || p.Key != "enTrailer01" {
		t.Fatalf("fr falls back to English %+v %v", p, err)
	}
	s.Pick(ctx, "movie", 11, "de", "tester")
	if f.calls != 2 {
		t.Fatalf("videos read %d times", f.calls)
	}
	if _, err := s.Pick(ctx, "tv", 12, "en", "tester"); !errors.Is(err, ErrNoTrailer) {
		t.Fatalf("no videos %v", err)
	}
	if _, err := s.Pick(ctx, "movie", 99, "en", "tester"); !errors.Is(err, ErrNoTrailer) {
		t.Fatalf("unknown title %v", err)
	}
	if _, err := s.Pick(ctx, "show", 11, "en", "tester"); !errors.Is(err, ErrNoTrailer) {
		t.Fatal("bad kind")
	}
}

func TestRefreshMarksBrokenAndPromotes(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	broken := map[string]bool{"deTrailer01": true}
	yt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Query().Get("url"), "https://www.youtube.com/watch?v=")
		mu.Lock()
		asked[key]++
		b := broken[key]
		mu.Unlock()
		if b {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer yt.Close()
	now := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	s := open(t, Options{Fetcher: &fakeTMDB{}, OEmbedURL: yt.URL, Pause: -1, Now: func() time.Time { return now }})
	ctx := context.Background()
	if p, _ := s.Pick(ctx, "movie", 11, "de", "tester"); p.Key != "deTrailer01" {
		t.Fatal(p)
	}
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.Pick(ctx, "movie", 11, "de", "tester")
	if err != nil || p.Key != "enTrailer01" || p.Status != StatusOK || p.CheckedAt == "" {
		t.Fatalf("promoted %+v %v", p, err)
	}
	st := s.Stats()
	if st.Checked != 2 || st.Broken != 1 || st.Replaced != 1 || st.LastRun == "" || st.Titles != 1 {
		t.Fatalf("stats %+v", st)
	}
	// A second run the same day re-checks nothing.
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if asked["enTrailer01"] != 1 {
		t.Fatalf("second run %v %+v", asked, s.Stats())
	}
	// An unknown answer (YouTube down) keeps the key.
	now = now.Add(25 * time.Hour)
	yt.Close()
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Pick(ctx, "movie", 11, "de", "tester"); p.Key != "enTrailer01" {
		t.Fatal("kept on unknown")
	}
}

func TestChecksCappedAndBackOffOn429(t *testing.T) {
	var mu sync.Mutex
	code := 200
	yt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(code)
	}))
	defer yt.Close()
	now := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	s := open(t, Options{Fetcher: &fakeTMDB{}, OEmbedURL: yt.URL, Pause: -1, DailyCap: 1, Now: func() time.Time { return now }})
	ctx := context.Background()
	s.Pick(ctx, "movie", 11, "de", "tester")
	s.Pick(ctx, "movie", 11, "fr", "tester")
	if did, _, _ := s.CheckNext(ctx); !did {
		t.Fatal("first check")
	}
	if did, _, _ := s.CheckNext(ctx); did {
		t.Fatal("cap of 1 a day")
	}
	now = now.Add(24 * time.Hour)
	mu.Lock()
	code = 429
	mu.Unlock()
	did, limited, _ := s.CheckNext(ctx)
	if !did || !limited || s.Stats().Broken != 0 || s.Stats().ChecksToday != 0 {
		t.Fatalf("429 %v %v %+v", did, limited, s.Stats())
	}
}

func requests(t *testing.T, s *Service, id int64) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT requests FROM titles WHERE kind='movie' AND tmdb_id=? AND lang='en'", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// One asker repeating a title counts once a day, and adds at most AskCap
// titles a day, so it cannot steer the oEmbed check list.
func TestAsksCountPerDistinctAskerWithCap(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := open(t, Options{Fetcher: &fakeTMDB{}, AskCap: 1, Now: func() time.Time { return now }})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		s.Pick(ctx, "movie", 11, "en", "attacker")
	}
	if n := requests(t, s, 11); n != 1 {
		t.Fatalf("repeats counted: %d", n)
	}
	s.Pick(ctx, "movie", 11, "en", "other")
	if n := requests(t, s, 11); n != 2 {
		t.Fatalf("distinct asker not counted: %d", n)
	}
	// Over the cap: a second title from the attacker adds nothing.
	s.db.Exec("INSERT INTO titles VALUES ('movie', 13, 'en', '[{\"key\":\"k13key13\"}]', ?, 0)", now.Unix())
	s.Pick(ctx, "movie", 13, "en", "attacker")
	if n := requests(t, s, 13); n != 0 {
		t.Fatalf("cap ignored: %d", n)
	}
	// Next day: counted again.
	now = now.Add(24 * time.Hour)
	s.Pick(ctx, "movie", 11, "en", "attacker")
	if n := requests(t, s, 11); n != 3 {
		t.Fatalf("next day: %d", n)
	}
}

// TMDB unreadable (budget spent): older stored videos still answer.
func TestPickServesStoredVideosWhenTMDBFails(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := open(t, Options{Fetcher: failTMDB{}, Now: func() time.Time { return now }})
	s.db.Exec("INSERT INTO titles VALUES ('movie', 11, 'en', '[{\"key\":\"oldKey0001\"}]', ?, 0)", now.Add(-30*24*time.Hour).Unix())
	p, err := s.Pick(context.Background(), "movie", 11, "en", "x")
	if err != nil || p.Key != "oldKey0001" {
		t.Fatalf("%+v %v", p, err)
	}
}

type failTMDB struct{}

func (failTMDB) Fetch(context.Context, model.Query) (model.Result, error) {
	return model.Result{}, &model.FetchError{Code: "upstream_budget", Status: 503}
}
