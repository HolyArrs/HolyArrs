package library

import (
	"context"
	"errors"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A big import that meets a metadata server that is down, busy or asking
// to slow down waits it out and tries the folders it could not bring in
// again, instead of failing them and leaving the person to run it again.

type adoptWaitKey struct{}

// adoptWaiter is one job's wait state, shared by every Adopt call under
// its ctx (a move adopts one title per call).
type adoptWaiter struct {
	tell   func(left int, next time.Duration)
	mu     sync.Mutex
	round  int
	waited time.Duration
	spent  bool
}

// WithAdoptWait makes an import or move run under ctx wait on the metadata
// server and try again, telling f how many folders are left and how long
// until the next try. Without it an import fails those folders at once.
// Once the waits run out the folders left fail at once, until one comes in.
func WithAdoptWait(ctx context.Context, f func(left int, next time.Duration)) context.Context {
	return context.WithValue(ctx, adoptWaitKey{}, &adoptWaiter{tell: f})
}

// adoptWaitable says a later try can bring the folder in: the server was
// down, busy, still fetching, or refused this install's own sign-in (it
// signs up again on its own). A wrong key of the owner's never waits.
func adoptWaitable(err error) bool {
	return err != nil && (MetadataDown(err) || errors.Is(err, metadata.ErrRateLimited) ||
		errors.Is(err, metadata.ErrServicePending) || errors.Is(err, metadata.ErrInstallRefused))
}

// adoptWaitSteps are the waits between tries while nothing comes in; a
// folder that comes in starts them over.
var adoptWaitSteps = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 10 * time.Minute, 10 * time.Minute}

// adoptWaitBudget is the most one job waits in all.
var adoptWaitBudget = 2 * time.Hour

// adoptWaitTick is how often a wait tells the page how long is left.
var adoptWaitTick = 15 * time.Second

// adoptWaitFor is the wait before try round: its step, or the server's own
// asked wait when that is longer (at most an hour).
func adoptWaitFor(round int, asked time.Duration) time.Duration {
	d := adoptWaitSteps[min(max(round, 0), len(adoptWaitSteps)-1)]
	if asked > d {
		d = min(asked, time.Hour)
	}
	return d
}

// serverAsked is how long the metadata server asked HolyArrs to wait, if
// it is asking right now.
var serverAsked = func(now time.Time) time.Duration {
	if r, ok := metadata.CurrentRefusal(now); ok {
		return r.Until.Sub(now)
	}
	return 0
}

// askedFor is the server's wait, counted only when a folder was refused
// for being too fast or not ready yet.
func askedFor(errs []error) time.Duration {
	for _, err := range errs {
		if errors.Is(err, metadata.ErrRateLimited) || errors.Is(err, metadata.ErrServicePending) {
			return max(serverAsked(time.Now()), 0)
		}
	}
	return 0
}

// got starts the waits over: a folder came in.
func (w *adoptWaiter) got() {
	w.mu.Lock()
	w.round, w.spent = 0, false
	w.mu.Unlock()
}

// plan is the next wait, or false once the steps or the budget ran out.
func (w *adoptWaiter) plan(asked time.Duration) (time.Duration, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.spent || w.round >= len(adoptWaitSteps) || w.waited >= adoptWaitBudget {
		w.spent = true
		return 0, false
	}
	d := min(adoptWaitFor(w.round, asked), adoptWaitBudget-w.waited)
	w.round++
	w.waited += d
	return d, true
}

// retryWaiting tries again, after a wait, the folders the metadata server
// kept out, while ctx asks for it. A folder whose title was already saved
// is never tried again.
func (s *Service) retryWaiting(ctx context.Context, lf mediafiles.LibraryFolder, choices []AdoptChoice, out []AdoptResult, errs []error, by domain.Actor) {
	w, ok := ctx.Value(adoptWaitKey{}).(*adoptWaiter)
	if !ok {
		return
	}
	done := 0
	for i, err := range errs {
		if err == nil {
			done++
			if out[i].TitleID != 0 {
				w.got()
			}
		}
	}
	for {
		var left []int
		var why []error
		for i, err := range errs {
			if adoptWaitable(err) && out[i].TitleID == 0 {
				left, why = append(left, i), append(why, err)
			}
		}
		if len(left) == 0 {
			return
		}
		d, ok := w.plan(askedFor(why))
		if !ok || !adoptPause(ctx, d, len(left), w.tell) {
			return
		}
		for _, i := range left {
			if ctx.Err() != nil {
				return
			}
			out[i], errs[i] = s.adoptChoice(ctx, lf, choices[i], by)
			if errs[i] == nil {
				done++
				w.got()
				ReportScan(ctx, done, len(choices))
			}
		}
	}
}

// adoptPause waits d, telling tell how long is left; false when ctx ended.
func adoptPause(ctx context.Context, d time.Duration, left int, tell func(int, time.Duration)) bool {
	end := time.Now().Add(d)
	for {
		rest := time.Until(end)
		if rest <= 0 {
			tell(left, 0)
			return ctx.Err() == nil
		}
		tell(left, rest)
		t := time.NewTimer(min(rest, adoptWaitTick))
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
	}
}
