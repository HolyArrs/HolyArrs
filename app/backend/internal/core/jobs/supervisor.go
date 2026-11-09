package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Off      State = "off"
	Starting State = "starting"
	Running  State = "running"
	Retrying State = "retrying"
	Failed   State = "failed"
)

var def atomic.Pointer[Registry]

// State is one of the five states a stage or linked app shows.
type State string

// Status is what the health page shows for one stage or linked app.
type Status struct {
	Key      string    `json:"key"`  // stage:<name> or link:<kind>:<id>
	Kind     string    `json:"kind"` // stage, source, downloader, subtitle_site, media_server
	Name     string    `json:"name"`
	State    State     `json:"state"`
	Reason   string    `json:"reason"` // plain words; empty when running
	Since    time.Time `json:"since"`
	Stuck    bool      `json:"stuck"`    // a pass has not returned past its deadline
	Restarts int       `json:"restarts"` // restarts after a crash or an early end
	// EverySeconds, LastPass and NextPass are a periodic loop's interval,
	// when its last pass ended and when the next one starts (Schedules page).
	EverySeconds int        `json:"every_s,omitempty"`
	LastPass     *time.Time `json:"last_pass,omitempty"`
	NextPass     *time.Time `json:"next_pass,omitempty"`
}

// Registry holds the states and runs the supervised loops.
type Registry struct {
	// BaseDelay is the first delay before a restart (default 1s); it
	// doubles on each crash in a row up to MaxDelay (default 5m).
	BaseDelay, MaxDelay time.Duration
	// Logf logs crashes (default log.Printf).
	Logf func(format string, args ...any)
	// OnChange is told about every change (tests, live updates). It runs
	// under the Registry's lock and must not call back into it.
	OnChange func(Status)

	mu    sync.Mutex
	items map[string]*Status
	wg    sync.WaitGroup
	also  []func()
}

// Loop is one named background loop.
type Loop struct {
	Name  string // short key, e.g. backups
	Label string // plain words, e.g. Backups
	// Every > 0 makes Run one pass, started every Every; zero makes Run a
	// loop that lasts until ctx ends and is started again if it ends early.
	Every time.Duration
	// Deadline caps one pass (default Every): its context ends then, and a
	// pass still running is shown stuck until it returns.
	Deadline time.Duration
	Run      func(ctx context.Context) error
}

// errCrash is a run that panicked.
type errCrash struct{ v any }

// NewRegistry makes an empty Registry.
func NewRegistry() *Registry { return &Registry{items: map[string]*Status{}} }

// SetDefault sets the Registry the package-level Guard reports to.
func SetDefault(r *Registry) { def.Store(r) }

// Default returns the Registry set by SetDefault (may be nil).
func Default() *Registry { return def.Load() }

// Set records the state of one stage or linked app.
func (r *Registry) Set(key, kind, name string, st State, reason string) {
	r.update(key, func(s *Status) {
		s.Kind, s.Name = kind, name
		s.State, s.Reason = st, reason
	})
}

// Update changes one Status in place (the time changes with the state).
func (r *Registry) Update(key string, f func(*Status)) { r.update(key, f) }

// Get returns one Status.
func (r *Registry) Get(key string) (Status, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.items[key]
	if !ok {
		return Status{}, false
	}
	return *s, true
}

// List returns every Status: stages first, then linked apps by kind and name.
func (r *Registry) List() []Status {
	r.mu.Lock()
	out := make([]Status, 0, len(r.items))
	for _, s := range r.items {
		out = append(out, *s)
	}
	r.mu.Unlock()
	rank := func(s Status) int {
		if s.Kind == "stage" {
			return 0
		}
		return 1
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Key < b.Key
	})
	return out
}

// Wait waits until every loop started by Go and every piece of work
// started by GoGuard has returned, then runs the AlsoWait waiters.
func (r *Registry) Wait() {
	r.wg.Wait()
	r.mu.Lock()
	also := append([]func(){}, r.also...)
	r.mu.Unlock()
	for _, f := range also {
		f()
	}
}

// AlsoWait adds a waiter Wait runs once the loops have returned (a
// service's own background work, such as the media-server batch).
func (r *Registry) AlsoWait(f func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.also = append(r.also, f)
}

// GoGuard runs Guard(name, f) in its own goroutine; Wait waits for it.
func (r *Registry) GoGuard(name string, f func()) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.Guard(name, f)
	}()
}

// StageKey is the Registry key of a stage.
func StageKey(name string) string { return "stage:" + name }

func (e errCrash) Error() string { return fmt.Sprintf("crashed: %T", e.v) }

// Go starts a supervised loop. It returns at once; ending ctx stops it, and
// Wait waits for it. The channel closes once the loop has returned.
func (r *Registry) Go(ctx context.Context, l Loop) <-chan struct{} {
	key := StageKey(l.Name)
	r.Set(key, "stage", l.Label, Starting, "")
	r.wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer r.wg.Done()
		defer close(done)
		defer r.Set(key, "stage", l.Label, Off, "")
		if l.Every > 0 {
			r.periodic(ctx, key, l)
			return
		}
		r.long(ctx, key, l)
	}()
	return done
}

// Guard runs one piece of background work (an Alert send, an Event hook) and
// catches a crash in it, marking stage name failed; a call that ends well
// puts a failed stage back to running.
func (r *Registry) Guard(name string, f func()) {
	key := StageKey(name)
	err := call(context.Background(), func(context.Context) error { f(); return nil })
	if err != nil {
		reason := r.crashed(name, err)
		r.update(key, func(s *Status) {
			if s.Kind == "" {
				s.Kind, s.Name = "stage", name
			}
			s.State, s.Reason = Failed, reason
		})
		return
	}
	if st, ok := r.Get(key); ok && st.State == Failed && !st.Stuck {
		r.update(key, func(s *Status) { s.State, s.Reason = Running, "" })
	}
}

// Guard is Registry.Guard on the default Registry; with none it still
// catches and logs the crash.
func Guard(name string, f func()) {
	if r := Default(); r != nil {
		r.Guard(name, f)
		return
	}
	if err := call(context.Background(), func(context.Context) error { f(); return nil }); err != nil {
		log.Printf("supervisor: %s %s", name, err.Error())
	}
}

func (r *Registry) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (r *Registry) delay(crashes int) time.Duration {
	d, top := r.BaseDelay, r.MaxDelay
	if d <= 0 {
		d = time.Second
	}
	if top <= 0 {
		top = 5 * time.Minute
	}
	for i := 1; i < crashes && d < top; i++ {
		d *= 2
	}
	return min(d, top)
}

func (r *Registry) update(key string, f func(*Status)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.items[key]
	if s == nil {
		s = &Status{Key: key}
		r.items[key] = s
	}
	was := s.State
	f(s)
	if s.State != was || s.Since.IsZero() {
		s.Since = time.Now().UTC()
	}
	if r.OnChange != nil {
		r.OnChange(*s)
	}
}

func call(ctx context.Context, f func(context.Context) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errCrash{p}
		}
	}()
	return f(ctx)
}

// crashed logs a crash by the loop's name and the panic's type only: the
// panic text may hold a secret.
func (r *Registry) crashed(name string, err error) string {
	r.logf("supervisor: %s %s; it starts again after a delay", name, err.Error())
	return "It crashed and starts again on its own. The log has the time it happened."
}

// sleep waits d or until ctx ends; false means ctx ended.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (r *Registry) long(ctx context.Context, key string, l Loop) {
	crashes := 0
	for ctx.Err() == nil {
		r.Set(key, "stage", l.Label, Running, "")
		began := time.Now()
		err := call(ctx, l.Run)
		if ctx.Err() != nil {
			return
		}
		if time.Since(began) > r.delay(1000) {
			crashes = 0 // it ran well for a while
		}
		crashes++
		reason := "It stopped on its own and starts again."
		if _, ok := err.(errCrash); ok {
			reason = r.crashed(l.Name, err)
		} else if err != nil {
			r.logf("supervisor: %s stopped: %v", l.Name, err)
			reason = "It stopped with an error and starts again. The log has the details."
		}
		r.update(key, func(s *Status) { s.State, s.Reason = Failed, reason; s.Restarts++ })
		r.Set(key, "stage", l.Label, Retrying, reason)
		if !sleep(ctx, r.delay(crashes)) {
			return
		}
	}
}

// stuckGrace is how long after its deadline a pass may take to return
// (tidying up after its context ended) before it shows as stuck.
func stuckGrace(deadline time.Duration) time.Duration { return min(deadline, 10*time.Second) }

func (r *Registry) periodic(ctx context.Context, key string, l Loop) {
	deadline := l.Deadline
	if deadline <= 0 {
		deadline = l.Every
	}
	crashes := 0
	for ctx.Err() == nil {
		pctx, cancel := context.WithTimeout(ctx, deadline)
		done := make(chan error, 1)
		go func() { done <- call(pctx, l.Run) }()
		var err error
		select {
		case err = <-done:
		case <-time.After(deadline + stuckGrace(deadline)):
			r.update(key, func(s *Status) {
				s.Stuck, s.State = true, Failed
				s.Reason = fmt.Sprintf("Stuck: one pass has not finished after %s. It is not started again until it does; restarting the app clears it.", deadline)
			})
			err = <-done // never a second run while the first has not returned
			r.update(key, func(s *Status) { s.Stuck = false })
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		wait := l.Every
		switch e := err.(type) {
		case nil:
			crashes = 0
			r.Set(key, "stage", l.Label, Running, "")
		case errCrash:
			crashes++
			reason := r.crashed(l.Name, e)
			r.update(key, func(s *Status) { s.State, s.Reason = Failed, reason; s.Restarts++ })
			r.Set(key, "stage", l.Label, Retrying, reason)
			wait = r.delay(crashes)
		default:
			r.logf("supervisor: %s pass failed: %v", l.Name, err)
			reason := "The last pass failed; it runs again at the next turn. The log has the details."
			if errors.Is(err, context.DeadlineExceeded) {
				reason = fmt.Sprintf("One pass took longer than %s and was stopped; it runs again at the next turn.", deadline)
			}
			r.Set(key, "stage", l.Label, Retrying, reason)
		}
		ended, next := time.Now().UTC(), time.Now().UTC().Add(wait)
		r.update(key, func(s *Status) { s.EverySeconds, s.LastPass, s.NextPass = int(l.Every.Seconds()), &ended, &next })
		if !sleep(ctx, wait) {
			return
		}
	}
}
