package library

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type countingMovies struct {
	movies  *metadata.Static
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *countingMovies) Search(ctx context.Context, query string, year int) ([]metadata.Movie, error) {
	return p.movies.Search(ctx, query, year)
}

func (p *countingMovies) Movie(ctx context.Context, id string) (metadata.Movie, error) {
	n := p.calls.Add(1)
	if n == 1 && p.started != nil {
		close(p.started)
		select {
		case <-p.release:
		case <-ctx.Done():
			return metadata.Movie{}, ctx.Err()
		}
	}
	return p.movies.Movie(ctx, id)
}

func addRefreshMovies(t *testing.T, s *database.Store, count int) *metadata.Static {
	t.Helper()
	addFolder(t, s)
	ms := make([]metadata.Movie, 0, count)
	for i := 1; i <= count; i++ {
		m := metadata.Movie{Name: "Movie " + string(rune('A'+i-1)), Year: 2026, IDs: map[string]string{"tmdb": fmt.Sprint(i)}}
		ms = append(ms, m)
		if _, err := AddMovie(context.Background(), s, m, AddOptions{}, nil, now, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	return &metadata.Static{Movies: ms}
}

func setMovieRefreshSchedule(t *testing.T, s *database.Store, enabled bool) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET enabled = ? WHERE kind = ?", enabled, KindRefreshTitles)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func waitRefreshJob(t *testing.T, s *database.Store, id int64) domain.Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var j domain.Job
		if err := database.Get(context.Background(), s.Read(), "jobs", id, &j); err == nil && (j.State == "done" || j.State == "failed" || j.State == "cancelled") {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d did not finish", id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestScheduledMovieRefreshChecksOffBeforeProviderWork(t *testing.T) {
	s := open(t)
	static := addRefreshMovies(t, s, 1)
	p := &countingMovies{movies: static}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	svc := &Service{Store: s, Queue: q, Metadata: p, Now: func() time.Time { return now }}
	svc.Register(q)
	setMovieRefreshSchedule(t, s, false)
	id, _, err := q.Enqueue(context.Background(), KindRefreshTitles, nil, map[string]any{"scheduled": true}, "schedule:"+KindRefreshTitles, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	j := waitRefreshJob(t, s, id)
	if p.calls.Load() != 0 || j.Result["stopped"] != "schedule_off" {
		t.Fatalf("calls=%d result=%v", p.calls.Load(), j.Result)
	}
}

func TestScheduledMovieRefreshStopsBetweenProviderRequestsButManualStillRuns(t *testing.T) {
	s := open(t)
	static := addRefreshMovies(t, s, 2)
	p := &countingMovies{movies: static, started: make(chan struct{}), release: make(chan struct{})}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	svc := &Service{Store: s, Queue: q, Metadata: p, Now: func() time.Time { return now }}
	svc.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	id, _, err := q.Enqueue(context.Background(), KindRefreshTitles, nil, map[string]any{"scheduled": true}, "schedule:"+KindRefreshTitles, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	<-p.started
	setMovieRefreshSchedule(t, s, false)
	close(p.release)
	j := waitRefreshJob(t, s, id)
	if p.calls.Load() != 1 || j.Result["stopped"] != "schedule_off" || j.Result["refreshed"] != float64(1) {
		t.Fatalf("scheduled calls=%d result=%v", p.calls.Load(), j.Result)
	}

	p.started, p.release = nil, nil
	manual, _, err := q.Enqueue(context.Background(), KindRefreshTitles, nil, map[string]any{"by_hand": true}, KindRefreshTitles+":by_hand", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	j = waitRefreshJob(t, s, manual)
	if p.calls.Load() != 3 || j.Result["refreshed"] != float64(2) {
		t.Fatalf("manual calls=%d result=%v", p.calls.Load(), j.Result)
	}
}

func TestShouldRefresh(t *testing.T) {
	at := func(ago time.Duration) *time.Time { x := now.Add(-ago); return &x }
	released := Dates{Cinema: d("2025-01-01"), Digital: d("2025-02-01"), Physical: d("2025-03-01")}
	recentPhys := Dates{Cinema: d("2026-06-01"), Digital: d("2026-09-01"), Physical: d("2026-10-01")}
	cinemas := Dates{Cinema: d("2026-10-01")}
	announced := Dates{Cinema: d("2027-05-01")}
	cases := []struct {
		name    string
		fetched *time.Time
		ds      Dates
		want    bool
		why     string
	}{
		{"never fetched", nil, released, true, "never fetched"},
		{"stale beats everything", at(181 * 24 * time.Hour), released, true, "last fetch over 180 days ago"},
		{"fresh beats announced", at(11 * time.Hour), announced, false, "last fetch under 12 hours ago"},
		{"announced", at(13 * time.Hour), announced, true, "announced"},
		{"no dates counts as announced", at(13 * time.Hour), Dates{}, true, "announced"},
		{"in cinemas", at(2 * 24 * time.Hour), cinemas, true, "in cinemas"},
		{"physical in the last 30 days", at(2 * 24 * time.Hour), recentPhys, true, "physical release in the last 30 days"},
		{"released long ago", at(30 * 24 * time.Hour), released, false, "released"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ti := mediafiles.Title{RefreshedAt: c.fetched, CinemaDate: c.ds.Cinema, DigitalDate: c.ds.Digital, PhysicalDate: c.ds.Physical}
			yes, why := ShouldRefresh(ti, now)
			if yes != c.want || why != c.why {
				t.Fatalf("got %v %q", yes, why)
			}
		})
	}
}

func TestChangedSince(t *testing.T) {
	got := changedSince(time.Date(2026, 10, 12, 14, 37, 0, 0, time.UTC))
	if !got.Equal(time.Date(2026, 10, 12, 13, 45, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
}

// fakeTMDB serves two movies and a changed list naming only the second.
func fakeTMDB(t *testing.T, gone *bool, mu *sync.Mutex) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/3/movie/changes":
			if r.URL.Query().Get("start_date") != "2026-10-10" {
				t.Errorf("changes from %s", r.URL.Query().Get("start_date"))
			}
			_, _ = w.Write([]byte(`{"results":[{"id":222},{"id":999}],"page":1,"total_pages":1}`))
		case "/3/movie/111":
			if *gone {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(`{"id":111,"title":"First","release_date":"2025-01-01","poster_path":"/first.jpg"}`))
		case "/3/movie/222":
			_, _ = w.Write([]byte(`{"id":222,"title":"Second Renamed","release_date":"2026-12-01","release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"release_date":"2026-12-01","type":3},{"release_date":"2026-12-20","type":4}]}]}}`))
		case "/w342/first.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg bytes"))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRefreshPlanChangedListDroppedAndArtwork(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	var gone bool
	var mu sync.Mutex
	srv := fakeTMDB(t, &gone, &mu)
	tm := &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "test-read-value", nil }, Gap: time.Millisecond, Images: srv.URL}
	cache := &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: &http.Client{}}
	svc := &Service{Store: s, Metadata: tm, Artwork: cache, Now: func() time.Time { return now }}
	first := metadata.Movie{Name: "First", Year: 2025, IDs: map[string]string{"tmdb": "111"}, CinemaDate: d("2025-01-01")}
	second := metadata.Movie{Name: "Second", Year: 2026, IDs: map[string]string{"tmdb": "222"}, CinemaDate: d("2026-12-01")}
	id1, _ := AddMovie(ctx, s, first, AddOptions{}, nil, now, domain.System)
	id2, _ := AddMovie(ctx, s, second, AddOptions{}, nil, now, domain.System)

	// Changed list in use (last start 3 days ago): only the second movie.
	last := time.Date(2026, 10, 10, 9, 5, 0, 0, time.UTC)
	ids, why, err := svc.RefreshPlan(ctx, false, &last)
	if err != nil || len(ids) != 1 || ids[0] != id2 || why[id2] != "on the changed list" {
		t.Fatalf("changed plan %v %v %v", ids, why, err)
	}
	// Last start too old for the list: the rules decide (both never fetched).
	old := now.Add(-20 * 24 * time.Hour)
	if ids, _, _ := svc.RefreshPlan(ctx, false, &old); len(ids) != 2 {
		t.Fatalf("rules plan %v", ids)
	}
	// By hand: every movie.
	if ids, _, _ := svc.RefreshPlan(ctx, true, nil); len(ids) != 2 {
		t.Fatalf("hand plan %v", ids)
	}

	out, err := svc.RefreshTitle(ctx, id2, domain.System, nil)
	if err != nil || out.GateDriver != "digital" || out.GateOpensAt == nil || !strings.HasPrefix(*out.GateOpensAt, "2026-12-20") {
		t.Fatalf("refresh second %+v %v", out, err)
	}
	var t2 mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id2, &t2)
	if t2.Name != "Second Renamed" || t2.DigitalDate == nil || t2.RefreshedAt == nil {
		t.Fatalf("second after refresh %+v", t2)
	}

	if _, err := svc.RefreshTitle(ctx, id1, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	var t1 mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id1, &t1)
	if len(t1.Artwork) != 1 || !strings.HasPrefix(t1.Artwork[0], "poster:") {
		t.Fatalf("artwork %v", t1.Artwork)
	}
	if p, ok := cache.Path(strings.TrimPrefix(t1.Artwork[0], "poster:")); !ok {
		t.Fatal("poster not cached")
	} else if b, _ := os.ReadFile(p); string(b) != "jpeg bytes" {
		t.Fatalf("poster %q", b)
	}

	// TMDB drops the first movie: flagged, kept, one Event.
	mu.Lock()
	gone = true
	mu.Unlock()
	for range 2 {
		out, err := svc.RefreshTitle(ctx, id1, domain.System, nil)
		if err != nil || !out.Dropped {
			t.Fatalf("dropped %+v %v", out, err)
		}
	}
	_ = database.Get(ctx, s.Read(), "titles", id1, &t1)
	if t1.DroppedAt == nil || t1.State != "active" {
		t.Fatalf("dropped title %+v", t1)
	}
	es, _ := s.RecentEvents(ctx, 50)
	n := 0
	for _, e := range es {
		if e.Kind == domain.EventTitleDropped {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d dropped events", n)
	}
}

func TestGateCrossingAtTheRightTime(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	clock := now
	svc := &Service{Store: s, Now: func() time.Time { return clock }}
	m := metadata.Movie{Name: "Soon", Year: 2026, IDs: map[string]string{"tmdb": "7"}, CinemaDate: d("2026-09-01"), DigitalDate: d("2026-10-20")}
	id, _ := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System)
	if slotsOf(t, s, id)[0].State != mediafiles.SlotNotOutYet {
		t.Fatal("should start not out yet")
	}
	writes := s.WriteCount()
	clock = time.Date(2026, 10, 19, 23, 59, 0, 0, time.UTC)
	if n, err := svc.OpenGates(ctx); err != nil || n != 0 {
		t.Fatalf("early: %d %v", n, err)
	}
	if s.WriteCount() != writes {
		t.Fatal("a gate check with nothing to open wrote to the database")
	}
	clock = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if n, err := svc.OpenGates(ctx); err != nil || n != 1 {
		t.Fatalf("on the day: %d %v", n, err)
	}
	sl := slotsOf(t, s, id)[0]
	if sl.State != mediafiles.SlotWanted || sl.LastReason == nil || !strings.Contains(*sl.LastReason, "2026-10-20 (digital date)") {
		t.Fatalf("slot %+v", sl)
	}
	es, _ := s.RecentEvents(ctx, 5)
	if es[0].Kind != domain.EventGateOpened {
		t.Fatalf("event %+v", es[0])
	}
}

func TestCalendarMonth(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	for i, m := range []metadata.Movie{
		{Name: "B", IDs: map[string]string{"tmdb": "1"}, CinemaDate: d("2026-11-05"), DigitalDate: d("2026-12-20"), PhysicalDate: d("2026-11-30")},
		{Name: "A", IDs: map[string]string{"tmdb": "2"}, DigitalDate: d("2026-11-05")},
		{Name: "C", IDs: map[string]string{"tmdb": "3"}, CinemaDate: d("2026-10-31")},
	} {
		if _, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System); err != nil {
			t.Fatal(i, err)
		}
	}
	es, err := Calendar(ctx, s.Read(), *d("2026-11-01"), *d("2026-12-01"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Date.Format("01-02")+" "+e.Name+" "+e.Kind)
	}
	want := "11-05 B cinema|11-05 A digital|11-30 B physical"
	if strings.Join(got, "|") != want {
		t.Fatalf("got %v", got)
	}
}

func TestArtworkCacheCapAndHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html")
		} else {
			w.Header().Set("Content-Type", "image/jpeg")
		}
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Cap: 250, Allowed: []string{srv.URL + "/img/"}, HTTP: &http.Client{}}
	ctx := context.Background()
	if _, err := c.Fetch(ctx, "http://elsewhere.test/img/a.jpg"); err != ErrArtworkURL {
		t.Fatalf("other host: %v", err)
	}
	if _, err := c.Fetch(ctx, srv.URL+"/img/../page"); err != ErrArtworkURL {
		t.Fatalf("dot-dot path: %v", err)
	}
	if _, err := (&ArtworkCache{Dir: c.Dir, Allowed: []string{srv.URL + "/"}, HTTP: &http.Client{}}).Fetch(ctx, srv.URL+"/page"); err == nil {
		t.Fatal("a page that is not an image was kept")
	}
	var names []string
	for _, p := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		n, err := c.Fetch(ctx, srv.URL+"/img/"+p)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
		old := time.Now().Add(-time.Duration(10-len(names)) * time.Hour)
		_ = os.Chtimes(filepath.Join(c.Dir, n), old, old)
	}
	if _, ok := c.Path(names[0]); ok {
		t.Fatal("the oldest picture should have been dropped to fit the cap")
	}
	for _, n := range names[1:] {
		if _, ok := c.Path(n); !ok {
			t.Fatalf("%s dropped", n)
		}
	}
	if _, ok := c.Path("../etc"); ok {
		t.Fatal("path outside the cache")
	}
}

// A lookup always stores a score, 0 when the source has none; only titles
// added before scores existed read back with none (NULL).
func TestMovieScoreStored(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	rated := dune
	rated.Score, rated.Votes = 8.1, 6200
	id1, err := AddMovie(ctx, s, rated, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var t1, t2 mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id1, &t1)
	_ = database.Get(ctx, s.Read(), "titles", id2, &t2)
	if t1.Score == nil || *t1.Score != 8.1 || t1.Votes != 6200 {
		t.Fatalf("rated %v %d", t1.Score, t1.Votes)
	}
	if t2.Score == nil || *t2.Score != 0 || t2.Votes != 0 {
		t.Fatalf("looked up without a score %v %d", t2.Score, t2.Votes)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET score = NULL WHERE id = ?", id2)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = database.Get(ctx, s.Read(), "titles", id2, &t2)
	if t2.Score != nil {
		t.Fatalf("NULL score read back as %v", *t2.Score)
	}
}

type changedMovies struct {
	*metadata.Static
	ids []string
}

func (c changedMovies) Changed(context.Context, time.Time) ([]string, error) { return c.ids, nil }

// Titles with no score yet are picked even while the changed list decides,
// at most ScoreFillPerRun a run, and never one fetched under 12 hours ago
// or one the source dropped.
func TestRefreshPlanFillsScores(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	total := ScoreFillPerRun + 5
	for i := 1; i <= total; i++ {
		m := metadata.Movie{Name: fmt.Sprint("Movie ", i), Year: 2020, IDs: map[string]string{"tmdb": fmt.Sprint(i)}, CinemaDate: d("2020-01-01")}
		if _, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	// Every movie was fetched 13 hours ago, before scores were kept; movie 1
	// was fetched 2 hours ago, movie 2 has a score, movie 3 was dropped.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		ago13, ago2 := database.FormatTime(now.Add(-13*time.Hour)), database.FormatTime(now.Add(-2*time.Hour))
		for _, u := range []struct {
			q   string
			arg any
		}{
			{"UPDATE titles SET score = NULL, refreshed_at = ?", ago13},
			{`UPDATE titles SET refreshed_at = ? WHERE json_extract(source_ids, '$.tmdb') = '1'`, ago2},
			{`UPDATE titles SET score = ? WHERE json_extract(source_ids, '$.tmdb') = '2'`, 7.4},
			{`UPDATE titles SET dropped_at = ? WHERE json_extract(source_ids, '$.tmdb') = '3'`, ago13},
		} {
			if _, err := tx.Exec(u.q, u.arg); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: s, Metadata: changedMovies{Static: &metadata.Static{}, ids: []string{"tmdb:2"}}, Now: func() time.Time { return now }}
	last := now.Add(-24 * time.Hour)
	ids, why, err := svc.RefreshPlan(ctx, false, &last)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	filled := 0
	for _, id := range ids {
		var tt mediafiles.Title
		_ = database.Get(ctx, s.Read(), "titles", id, &tt)
		byID[tt.SourceIDs["tmdb"]] = why[id]
		if why[id] == "no score yet" {
			filled++
		}
	}
	if filled != ScoreFillPerRun || byID["2"] != "on the changed list" || byID["1"] != "" || byID["3"] != "" || byID["4"] != "no score yet" {
		t.Fatalf("filled %d, why 1 %q 2 %q 3 %q 4 %q", filled, byID["1"], byID["2"], byID["3"], byID["4"])
	}
	if ids, _, _ := svc.RefreshPlan(ctx, true, nil); len(ids) != total {
		t.Fatalf("by hand %d", len(ids))
	}
}
