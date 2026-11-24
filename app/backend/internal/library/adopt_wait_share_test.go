package library

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestAdoptWaiterPlan(t *testing.T) {
	steps, budget := adoptWaitSteps, adoptWaitBudget
	t.Cleanup(func() { adoptWaitSteps, adoptWaitBudget = steps, budget })
	adoptWaitSteps = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute}
	for _, c := range []struct {
		name   string
		budget time.Duration
		asked  time.Duration
		gotAt  int // before this plan a folder comes in (-1: never)
		want   []time.Duration
	}{
		{"the steps, then no more", time.Hour, 0, -1, []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 0, 0}},
		{"a folder coming in starts them over", time.Hour, 0, 2, []time.Duration{time.Minute, 2 * time.Minute, time.Minute, 2 * time.Minute, 5 * time.Minute, 0}},
		{"the budget cuts the last wait and then stops", 4 * time.Minute, 0, -1, []time.Duration{time.Minute, 2 * time.Minute, time.Minute, 0}},
		{"the budget holds even when folders come in", 3 * time.Minute, 0, 1, []time.Duration{time.Minute, time.Minute, time.Minute, 0}},
		{"a longer asked wait is used, within the budget", 10 * time.Minute, 7 * time.Minute, -1, []time.Duration{7 * time.Minute, 3 * time.Minute, 0}},
	} {
		t.Run(c.name, func(t *testing.T) {
			adoptWaitBudget = c.budget
			w := &adoptWaiter{}
			for i, want := range c.want {
				if i == c.gotAt {
					w.got()
				}
				d, ok := w.plan(c.asked)
				if d != want || ok != (want > 0) {
					t.Fatalf("plan %d = %v %v, want %v", i, d, ok, want)
				}
			}
		})
	}
}

func TestAskedFor(t *testing.T) {
	asked := serverAsked
	t.Cleanup(func() { serverAsked = asked })
	serverAsked = func(time.Time) time.Duration { return 9 * time.Minute }
	for _, c := range []struct {
		name string
		errs []error
		want time.Duration
	}{
		{"too fast: the server's wait", []error{metadata.ErrRateLimited}, 9 * time.Minute},
		{"not ready yet: the server's wait", []error{fmt.Errorf("x: %w", metadata.ErrServicePending)}, 9 * time.Minute},
		{"down: never an old refusal", []error{metadata.ErrServiceUnavailable}, 0},
		{"install refused: steps only", []error{metadata.InstallRefused(metadata.ErrNoAccess)}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := askedFor(c.errs); got != c.want {
				t.Fatalf("askedFor = %v, want %v", got, c.want)
			}
		})
	}
}

// A move adopts one title per call under one job: once the waits ran out,
// the next titles fail at once; a title that comes in starts them over.
func TestAdoptWaitSharedAcrossCalls(t *testing.T) {
	shortWaits(t)
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	md := &flakyMovies{Static: adoptTMDB, err: metadata.ErrServiceUnavailable}
	svc := &Service{Store: s, Metadata: md, Now: func() time.Time { return now }}
	waits := 0
	ctx := WithAdoptWait(context.Background(), func(_ int, next time.Duration) {
		if next > 0 {
			waits++
		}
	})
	for _, c := range []struct {
		name      string
		folder    string
		tmdb      string
		fails     int
		wantWaits bool
		wantIn    bool
	}{
		{"first title waits out the steps and fails", "Heat (1995) {tmdb-949}", "949", 1000, true, false},
		{"next title fails at once", "Heat (1995) {tmdb-949}", "949", 1000, false, false},
		{"the server answers: the title comes in", "Heat (1995) {tmdb-949}", "949", 0, false, true},
		{"after a title came in, waiting starts over", "Dune - Part Two (2024)", "693134", 1000, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			md.fails, waits = c.fails, 0
			res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: c.folder, TMDB: c.tmdb}}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			if (waits > 0) != c.wantWaits || (res[0].Error == "") != c.wantIn {
				t.Fatalf("waits %d, result %+v", waits, res[0])
			}
		})
	}
}

// A title already saved (an artist whose albums could not be read) keeps
// its result and is never tried again.
func TestRetrySkipsSavedTitles(t *testing.T) {
	shortWaits(t)
	ctx := WithAdoptWait(context.Background(), func(int, time.Duration) { t.Error("waited for a saved title") })
	out := []AdoptResult{{Folder: "Artist", TitleID: 7}}
	errs := []error{fmt.Errorf("the artist was added but its albums could not be read (%w)", metadata.ErrServiceUnavailable)}
	(&Service{}).retryWaiting(ctx, mediafiles.LibraryFolder{}, []AdoptChoice{{Folder: "Artist"}}, out, errs, domain.System)
	if out[0].TitleID != 7 || errs[0] == nil {
		t.Fatalf("a saved title was changed: %+v %v", out[0], errs[0])
	}
}

// Each folder that comes in on a later try moves the count.
func TestRetryMovesTheCount(t *testing.T) {
	shortWaits(t)
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	md := &flakyMovies{Static: adoptTMDB, fails: 2, err: metadata.ErrServiceBusy}
	svc := &Service{Store: s, Metadata: md, Now: func() time.Time { return now }}
	var seen []int
	ctx := WithScanProgress(WithAdoptWait(context.Background(), func(int, time.Duration) {}), func(done, _ int) { seen = append(seen, done) })
	if _, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}, {Folder: "Dune - Part Two (2024)", TMDB: "693134"}}, domain.System); err != nil {
		t.Fatal(err)
	}
	// The first pass reads 0 and 1; each retried folder that comes in reports again.
	if !slices.Equal(seen, []int{0, 1, 1, 2, 2}) {
		t.Fatalf("the count never showed a retried folder coming in: %v", seen)
	}
}
