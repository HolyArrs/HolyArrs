package library

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// refusingMovies answers the static provider unless ask says otherwise for
// the nth lookup: a metadata server that refuses, or does not know, one
// movie now and answers the next ones.
type refusingMovies struct {
	static *metadata.Static
	ask    func(n int, id string) error
	mu     sync.Mutex
	n      int
}

func (p *refusingMovies) Search(ctx context.Context, query string, year int) ([]metadata.Movie, error) {
	return p.static.Search(ctx, query, year)
}

func (p *refusingMovies) Movie(ctx context.Context, id string) (metadata.Movie, error) {
	p.mu.Lock()
	p.n++
	n := p.n
	p.mu.Unlock()
	if p.ask != nil {
		if err := p.ask(n, id); err != nil {
			return metadata.Movie{}, err
		}
	}
	return p.static.Movie(ctx, id)
}

func importedMovies(t *testing.T, s *database.Store, count int) *metadata.Static {
	t.Helper()
	addFolder(t, s)
	ms := make([]metadata.Movie, 0, count)
	for i := 1; i <= count; i++ {
		m := metadata.Movie{Name: "Imported " + string(rune('A'+i-1)), Year: 2020, IDs: map[string]string{"tmdb": strconv.Itoa(i)}, CinemaDate: d("2020-01-01")}
		if _, err := AddMovie(context.Background(), s, m, AddOptions{}, nil, now, domain.System); err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	return &metadata.Static{Movies: ms}
}

func runRefreshTitles(t *testing.T, s *database.Store, p metadata.Provider) domain.Job {
	t.Helper()
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, MaxAttempts: 2, BaseBackoff: 10 * time.Millisecond})
	svc := &Service{Store: s, Queue: q, Metadata: p, Now: func() time.Time { return now }}
	svc.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	id, _, err := q.Enqueue(context.Background(), KindRefreshTitles, nil, map[string]any{"by_hand": true}, KindRefreshTitles+":by_hand", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return waitRefreshJob(t, s, id)
}

func titleBy(t *testing.T, s *database.Store, titleID int64) mediafiles.Title {
	t.Helper()
	var ti mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", titleID, &ti); err != nil {
		t.Fatal(err)
	}
	return ti
}

func planHas(t *testing.T, svc *Service, id int64) (bool, string) {
	t.Helper()
	ids, why, err := svc.RefreshPlan(context.Background(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range ids {
		if got == id {
			return true, why[got]
		}
	}
	return false, ""
}

// A lookup that answered "not found" fetched nothing: recording it as a
// fetch left the movie without a poster and out of the "never fetched"
// rule for good (the reported bug: only a delete and re-add brought art).
func TestRefreshNotFoundIsNotAFetch(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	provider := metadata.Static{}
	svc := &Service{Store: s, Metadata: &provider, Now: func() time.Time { return now }}
	m := metadata.Movie{Name: "Gone", Year: 2020, IDs: map[string]string{"tmdb": "404"}, CinemaDate: d("2020-01-01")}
	id, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}

	out, err := svc.RefreshTitle(ctx, id, domain.System, nil)
	if err != nil || !out.Dropped {
		t.Fatalf("out %+v err %v", out, err)
	}
	ti := titleBy(t, s, id)
	if ti.DroppedAt == nil {
		t.Fatal("a movie the source does not know is not flagged")
	}
	if ti.RefreshedAt != nil {
		t.Fatalf("a lookup that answered not found was recorded as a fetch at %v", ti.RefreshedAt.Format(time.RFC3339))
	}
	if in, why := planHas(t, svc, id); !in || why != "never fetched" {
		t.Fatalf("the plan does not pick it up again: in=%v why=%q", in, why)
	}

	// A movie fetched before keeps its own fetch time when a later lookup
	// fails: the failure is not a fetch either.
	old := now.Add(-48 * time.Hour)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET refreshed_at = ? WHERE id = ?", database.FormatTime(old), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshTitle(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	ti = titleBy(t, s, id)
	if ti.RefreshedAt == nil || !ti.RefreshedAt.Equal(old) {
		t.Fatalf("a failed lookup moved refreshed_at from %v to %v", old, ti.RefreshedAt)
	}
}

// One refused lookup (a dropped install sign-in, made again by itself)
// must not end the run: the movies after it are still refreshed.
func TestRefreshRunGoesOnAfterOneRefusedLookup(t *testing.T) {
	s := open(t)
	static := importedMovies(t, s, 3)
	p := &refusingMovies{static: static, ask: func(n int, id string) error {
		if id == "tmdb:1" {
			return metadata.ErrNoAccess
		}
		return nil
	}}
	j := runRefreshTitles(t, s, p)
	if j.State != "done" {
		t.Fatalf("the run ended %q on one refused lookup: %v", j.State, j.Result)
	}
	if j.Result["refreshed"] != float64(2) {
		t.Fatalf("refreshed %v of 3 after one refusal: %v", j.Result["refreshed"], j.Result)
	}
	if failed, _ := j.Result["failed"].([]any); len(failed) != 1 {
		t.Fatalf("failed %v", j.Result["failed"])
	}
	for i := 2; i <= 3; i++ {
		if ti := titleBy(t, s, int64(i)); ti.RefreshedAt == nil {
			t.Fatalf("movie %d was not refreshed after the refusal", i)
		}
	}
}

// A server that refuses every lookup pauses the run where it is instead
// of failing it: the movies it never came to keep qualifying, so the next
// run takes them, and the result says how many were left.
func TestRefreshRunPausesWhenEveryLookupIsRefused(t *testing.T) {
	s := open(t)
	static := importedMovies(t, s, 5)
	p := &refusingMovies{static: static, ask: func(int, string) error { return metadata.ErrNoAccess }}
	j := runRefreshTitles(t, s, p)
	if j.State != "done" {
		t.Fatalf("the run ended %q instead of pausing: %v", j.State, j.Result)
	}
	failed, _ := j.Result["failed"].([]any)
	if j.Result["stopped"] != "metadata_no_access" || j.Result["refreshed"] != float64(0) ||
		len(failed) != 3 || j.Result["paused"] != float64(2) {
		t.Fatalf("result %v", j.Result)
	}
	svc := &Service{Store: s, Metadata: static, Now: func() time.Time { return now }}
	for i := 1; i <= 5; i++ {
		if in, why := planHas(t, svc, int64(i)); !in || why != "never fetched" {
			t.Fatalf("movie %d is not left for the next run: in=%v why=%q", i, in, why)
		}
	}
}
