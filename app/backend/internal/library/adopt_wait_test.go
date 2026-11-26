package library

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestAdoptWaitFor(t *testing.T) {
	for _, c := range []struct {
		name  string
		round int
		asked time.Duration
		want  time.Duration
	}{
		{"first wait", 0, 0, time.Minute},
		{"second wait", 1, 0, 2 * time.Minute},
		{"past the last step stays at it", 40, 0, 10 * time.Minute},
		{"a negative round is the first", -1, 0, time.Minute},
		{"the server's longer Retry-After wins", 0, 7 * time.Minute, 7 * time.Minute},
		{"a shorter Retry-After keeps the step", 3, time.Minute, 10 * time.Minute},
		{"a huge Retry-After is cut to an hour", 0, 5 * time.Hour, time.Hour},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := adoptWaitFor(c.round, c.asked); got != c.want {
				t.Fatalf("adoptWaitFor(%d, %v) = %v, want %v", c.round, c.asked, got, c.want)
			}
		})
	}
}

func TestAdoptWaitable(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want bool
	}{
		{"none", nil, false},
		{"server down", fmt.Errorf("TMDB 1: %w", metadata.ErrServiceUnavailable), true},
		{"server busy", metadata.ErrServiceBusy, true},
		{"rate limited", fmt.Errorf("x: %w", metadata.ErrRateLimited), true},
		{"still fetching", metadata.ErrServicePending, true},
		{"install sign-in refused (signs up again)", fmt.Errorf("TMDB 2: %w", metadata.InstallRefused(metadata.ErrNoAccess)), true},
		{"hosted sign-in refused", metadata.InstallRefused(metadata.ErrServiceAccess), true},
		{"owner's wrong key", fmt.Errorf("TMDB 2: %w", metadata.ErrNoAccess), false},
		{"sign-in could not be read", metadata.ErrServiceAccess, false},
		{"not found", metadata.ErrNotFound, false},
		{"anything else", errors.New("no such folder in the Library folder"), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := adoptWaitable(c.err); got != c.want {
				t.Fatalf("adoptWaitable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestWaitNote(t *testing.T) {
	for _, c := range []struct {
		done, total, left int
		next              time.Duration
		want              string
	}{
		{12, 50, 38, 10 * time.Minute, "12/50 wait 600 38"},
		{0, 3, 3, 1500 * time.Millisecond, "0/3 wait 2 3"},
		{4, 4, 0, 0, "4/4 wait 0 0"},
	} {
		if got := WaitNote(c.done, c.total, c.next, c.left); got != c.want {
			t.Fatalf("WaitNote = %q, want %q", got, c.want)
		}
	}
}

// flakyMovies answers like the server that is down or asks to wait for the
// first calls, then answers.
type flakyMovies struct {
	*metadata.Static
	mu    sync.Mutex
	fails int
	err   error
}

func (f *flakyMovies) Movie(ctx context.Context, id string) (metadata.Movie, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return metadata.Movie{}, fmt.Errorf("TMDB: %w", f.err)
	}
	return f.Static.Movie(ctx, id)
}

func shortWaits(t *testing.T) {
	steps, tick, asked, budget := adoptWaitSteps, adoptWaitTick, serverAsked, adoptWaitBudget
	adoptWaitSteps, adoptWaitTick = []time.Duration{time.Millisecond, time.Millisecond}, time.Millisecond
	serverAsked = func(time.Time) time.Duration { return 0 }
	t.Cleanup(func() { adoptWaitSteps, adoptWaitTick, serverAsked, adoptWaitBudget = steps, tick, asked, budget })
}

func TestAdoptWaitsAndRetries(t *testing.T) {
	shortWaits(t)
	for _, c := range []struct {
		name     string
		fails    int
		err      error
		wait     bool
		wantIn   bool
		wantWhy  string
		wantTold bool
	}{
		{"down, then answers: brought in on a later try", 2, metadata.ErrServiceUnavailable, true, true, "", true},
		{"asks to slow down, then answers", 1, metadata.ErrServiceBusy, true, true, "", true},
		{"install sign-in refused, then answers", 1, metadata.InstallRefused(metadata.ErrNoAccess), true, true, "", true},
		{"owner's wrong key fails at once", 1, metadata.ErrNoAccess, true, false, "", false},
		{"never answers: fails with its reason after the last wait", 100, metadata.ErrServiceUnavailable, true, false, MetadataNotAnswering, true},
		{"without the wait (not a job) it fails at once", 1, metadata.ErrServiceUnavailable, false, false, MetadataNotAnswering, false},
		{"not found is never waited for", 1, metadata.ErrNotFound, true, false, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := open(t)
			lf := addFolder(t, s)
			adoptTree(t, lf.Path)
			md := &flakyMovies{Static: adoptTMDB, fails: c.fails, err: c.err}
			svc := &Service{Store: s, Metadata: md, Now: func() time.Time { return now }}
			told := false
			ctx := context.Background()
			if c.wait {
				ctx = WithAdoptWait(ctx, func(left int, _ time.Duration) {
					told = true
					if left != 1 {
						t.Errorf("folders left = %d, want 1", left)
					}
				})
			}
			res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			if in := res[0].TitleID != 0 && res[0].Error == ""; in != c.wantIn {
				t.Fatalf("brought in = %v, want %v (%q)", in, c.wantIn, res[0].Error)
			}
			if c.wantWhy != "" && res[0].Error != c.wantWhy {
				t.Fatalf("why = %q, want %q", res[0].Error, c.wantWhy)
			}
			if told != c.wantTold {
				t.Fatalf("wait told = %v, want %v", told, c.wantTold)
			}
		})
	}
}

// Waiting stops as soon as the import is cancelled.
func TestAdoptPauseStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if adoptPause(ctx, time.Hour, 3, func(int, time.Duration) {}) {
		t.Fatal("a cancelled wait went on")
	}
}
