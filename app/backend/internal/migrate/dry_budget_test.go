package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// slowShows answers like the metadata server, a few milliseconds a lookup.
type slowShows struct {
	mu    sync.Mutex
	by    map[string]metadata.Show
	calls int
	wait  time.Duration
}

func (s *slowShows) SearchShows(context.Context, string) ([]metadata.Show, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	time.Sleep(s.wait)
	return nil, nil
}

func (s *slowShows) Show(_ context.Context, id string) (metadata.Show, error) {
	s.mu.Lock()
	s.calls++
	sh, ok := s.by[id]
	s.mu.Unlock()
	time.Sleep(s.wait)
	if !ok {
		return metadata.Show{}, metadata.ErrShowNotFound
	}
	return sh, nil
}

func (s *slowShows) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// A dry run of 2,000 new shows answers within its time limit, saying how
// many shows are still being checked; the rest are looked up after it
// answers, and the next dry run reuses every answer instead of asking again.
func TestDryRunLookupsCappedAndKept(t *testing.T) {
	old := dryLookupBudget
	dryLookupBudget = 300 * time.Millisecond
	t.Cleanup(func() { dryLookupBudget = old })
	l := newMoveLab(t)
	tv := filepath.Join(t.TempDir(), "tv")
	src := &slowShows{by: map[string]metadata.Show{}, wait: 2 * time.Millisecond}
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, Folders: []Folder{{Path: "/tv"}}}
	const n = 2000
	for i := 1; i <= n; i++ {
		s := strconv.Itoa(i)
		src.by["tvdb:"+s] = metadata.Show{Source: "tmdb", Name: "Show " + s, IDs: map[string]string{"tmdb": "9" + s, "tvdb": s}}
		sonarr.Titles = append(sonarr.Titles, Title{SourceID: int64(i), Kind: "show", Name: "Show " + s, IDs: map[string]string{"tvdb": s, "tmdb": "9" + s},
			Path: "/tv/Show " + s, Monitored: true})
		if err := os.MkdirAll(filepath.Join(tv, "Show "+s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": src}
	sonarr.finish()
	saved, err := Save(context.Background(), l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	l.rules = append(l.rules, PathRule{From: "/tv", To: filepath.ToSlash(tv)})
	p := l.plan(t)
	by := domain.Actor{Kind: "user", ID: 1}
	start := time.Now()
	rep := l.mv.Run(context.Background(), 0, p, true, by, nil)
	took := time.Since(start)
	t.Logf("first dry run: %v, %d lookups, %d still being checked", took, src.count(), rep.Checking)
	if rep.Checking == 0 || rep.Checking >= n {
		t.Fatalf("first dry run checked %d of %d shows in time", n-rep.Checking, n)
	}
	// Each show's own id is looked up at most once, whatever runs it.
	deadline := time.Now().Add(30 * time.Second)
	for l.mv.dryWarming() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	before := src.count()
	start = time.Now()
	rep = l.mv.Run(context.Background(), 0, p, true, by, nil)
	t.Logf("second dry run: %v, %d new lookups, %d still being checked; %d lookups in all", time.Since(start), src.count()-before, rep.Checking, src.count())
	if rep.Checking != 0 || src.count() != before || src.count() > n {
		t.Fatalf("second dry run: %d still checked, %d new lookups, %d in all", rep.Checking, src.count()-before, src.count())
	}
}

// laggyShows answers as manyShows does, each answer late.
type laggyShows struct {
	manyShows
	wait time.Duration
}

func (s *laggyShows) Show(ctx context.Context, id string) (metadata.Show, error) {
	time.Sleep(s.wait)
	return s.manyShows.Show(ctx, id)
}

// A show whose seasons differ that the test run could not check in time is
// counted as still being checked, and the import itself checks it: it is
// never brought over as an ordinary show with its files on episodes by number.
func TestUncheckedShowStillChecked(t *testing.T) {
	old := dryLookupBudget
	dryLookupBudget = time.Millisecond
	t.Cleanup(func() { dryLookupBudget = old })
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()}, cunkFiles...)
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": &laggyShows{manyShows: manyShows{list: []metadata.Show{cunkOn()}}, wait: 20 * time.Millisecond}}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if dry.Checking != 1 || lineOf(t, dry, cunkKey).Attention == AttnSeasons {
		t.Fatalf("test run: %d still being checked, line %+v", dry.Checking, lineOf(t, dry, cunkKey))
	}
	got := lineOf(t, l.move(t, Choices{}).Report, cunkKey)
	if got.Attention != AttnSeasons || l.count(t, `SELECT count(*) FROM titles WHERE seasons_differ = 1`) != 1 || l.count(t, showFilled) != 0 {
		t.Fatalf("import %+v; marked %d, filled %d", got, l.count(t, `SELECT count(*) FROM titles WHERE seasons_differ = 1`), l.count(t, showFilled))
	}
}

// stuckShows answers only when the caller gives up, like a metadata server
// that has stopped answering.
type stuckShows struct{ manyShows }

func (s *stuckShows) Show(ctx context.Context, _ string) (metadata.Show, error) {
	select {
	case <-ctx.Done():
		return metadata.Show{}, ctx.Err()
	case <-time.After(10 * time.Second):
		return metadata.Show{}, errors.New("too late")
	}
}

// a lookup already running when the test run's time ends is cut off
// there, so a stuck metadata server cannot hold the page past its budget.
func TestDryRunLookupStopsAtBudget(t *testing.T) {
	old := dryLookupBudget
	dryLookupBudget = 200 * time.Millisecond
	t.Cleanup(func() { dryLookupBudget = old })
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()}, cunkFiles...)
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": &stuckShows{manyShows: manyShows{list: []metadata.Show{cunkOn()}}}}
	start := time.Now()
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if took := time.Since(start); took > 3*time.Second || dry.Checking != 1 {
		t.Fatalf("test run took %v, %d still being checked", took, dry.Checking)
	}
}

// stuckSearch knows no show by id, and its name search answers only when
// the caller gives up (the search path ran past the time limit).
type stuckSearch struct{ manyShows }

func (s *stuckSearch) SearchShows(ctx context.Context, _ string) ([]metadata.Show, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, errors.New("too late")
	}
}

// A name search already running when the test run's time ends is cut off
// there too.
func TestDryRunNameSearchStopsAtBudget(t *testing.T) {
	old := dryLookupBudget
	dryLookupBudget = 200 * time.Millisecond
	t.Cleanup(func() { dryLookupBudget = old })
	l, _ := cunkSetup(t, cunkSonarr, nil, cunkFiles...)
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": &stuckSearch{}, "tvmaze": &stuckSearch{}, "tvdb": &stuckSearch{}}
	start := time.Now()
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	// A search cut off by the time limit is "still being checked", never "not found".
	if took := time.Since(start); took > 3*time.Second || dry.Checking != 1 || lineOf(t, dry, cunkKey).Result == ResFailed {
		t.Fatalf("test run took %v, %d still being checked, line %+v", took, dry.Checking, lineOf(t, dry, cunkKey))
	}
}

// A show the old app knows only by its TVDB id, whose name search the time
// limit cut off, is still being checked: the test run never calls it missing.
func TestDryRunCutSearchNotMissing(t *testing.T) {
	old := dryLookupBudget
	dryLookupBudget = 200 * time.Millisecond
	t.Cleanup(func() { dryLookupBudget = old })
	l, _ := cunkSetup(t, map[string]string{"tvdb": "414217"}, nil, cunkFiles...)
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": &stuckSearch{}, "tvmaze": &stuckSearch{}, "tvdb": &stuckSearch{}}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if dry.Checking != 1 || lineOf(t, dry, cunkKey).Result == ResFailed {
		t.Fatalf("%d still being checked, line %+v", dry.Checking, lineOf(t, dry, cunkKey))
	}
}
