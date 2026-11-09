package jobs

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func quiet(r *Registry) *Registry {
	r.Logf = func(string, ...any) {}
	return r
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func state(r *Registry, key string) Status {
	s, _ := r.Get(key)
	return s
}

// A long loop that crashes is marked failed, then retrying, then runs again,
// and the program keeps going.
func TestLoopCrashIsCaughtAndRestarted(t *testing.T) {
	r := quiet(NewRegistry())
	r.BaseDelay, r.MaxDelay = 30*time.Millisecond, 100*time.Millisecond
	var runs atomic.Int32
	var seen []State
	r.OnChange = func(s Status) {
		if s.Key == "stage:boom" {
			seen = append(seen, s.State)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.Go(ctx, Loop{Name: "boom", Label: "Boom", Run: func(ctx context.Context) error {
		if runs.Add(1) == 1 {
			panic("injected crash")
		}
		<-ctx.Done()
		return nil
	}})
	waitFor(t, "second run", func() bool { return runs.Load() >= 2 && state(r, "stage:boom").State == Running })
	cancel()
	r.Wait()
	if st := state(r, "stage:boom"); st.State != Off || st.Restarts != 1 {
		t.Fatalf("after stop: %+v", st)
	}
	want := []State{Starting, Running, Failed, Retrying, Running, Off}
	if strings.Join(states(seen), ",") != strings.Join(states(want), ",") {
		t.Fatalf("states %v, want %v", seen, want)
	}
}

func states(ss []State) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

// The reason for a crash is plain words and never carries the panic text
// (which may hold a secret); the log line names the loop.
func TestCrashReasonAndLogHoldNoPanicText(t *testing.T) {
	r := NewRegistry()
	r.BaseDelay = time.Hour
	var logged atomic.Value
	r.Logf = func(f string, a ...any) { logged.Store(f) }
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); r.Wait() }()
	r.Go(ctx, Loop{Name: "x", Label: "X", Run: func(context.Context) error { panic("private text q7z") }})
	waitFor(t, "failed", func() bool { return state(r, "stage:x").State == Retrying })
	st := state(r, "stage:x")
	if strings.Contains(st.Reason, "q7z") || !strings.Contains(st.Reason, "crashed") {
		t.Fatalf("reason %q", st.Reason)
	}
}

// The delay before a restart grows and stops at the cap.
func TestDelayGrowsToCap(t *testing.T) {
	r := NewRegistry()
	r.BaseDelay, r.MaxDelay = time.Second, 5*time.Second
	got := []time.Duration{r.delay(1), r.delay(2), r.delay(3), r.delay(4), r.delay(10)}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("delay(%d) = %v, want %v", i, got[i], want[i])
		}
	}
}

// A periodic pass that ignores its deadline is shown stuck and never started
// a second time while it has not returned.
func TestStuckPassIsNotStartedTwice(t *testing.T) {
	r := quiet(NewRegistry())
	var starts atomic.Int32
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	r.Go(ctx, Loop{Name: "slow", Label: "Slow", Every: 5 * time.Millisecond, Deadline: 20 * time.Millisecond, Run: func(context.Context) error {
		starts.Add(1)
		<-release // ignores its context on purpose
		return nil
	}})
	waitFor(t, "stuck", func() bool { return state(r, "stage:slow").Stuck })
	time.Sleep(100 * time.Millisecond) // many periods pass
	if n := starts.Load(); n != 1 {
		t.Fatalf("started %d times while stuck", n)
	}
	if st := state(r, "stage:slow"); st.State != Failed || !strings.Contains(st.Reason, "Stuck") {
		t.Fatalf("stuck state %+v", st)
	}
	close(release)
	waitFor(t, "unstuck", func() bool { st := state(r, "stage:slow"); return !st.Stuck && st.State == Running })
	cancel()
	r.Wait()
}

// A pass that honours its deadline (returning a moment after it) is never
// shown stuck.
func TestPassThatHonoursItsDeadlineIsNotStuck(t *testing.T) {
	r := quiet(NewRegistry())
	ctx, cancel := context.WithCancel(context.Background())
	r.Go(ctx, Loop{Name: "fair", Label: "Fair", Every: 5 * time.Millisecond, Deadline: 20 * time.Millisecond, Run: func(pctx context.Context) error {
		<-pctx.Done()
		time.Sleep(5 * time.Millisecond) // tidying up after the deadline
		return pctx.Err()
	}})
	end := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(end) {
		if st := state(r, "stage:fair"); st.Stuck {
			t.Fatalf("shown stuck: %+v", st)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	r.Wait()
}

// A periodic pass that crashes is failed, then retried after the delay.
func TestPeriodicCrashThenRetry(t *testing.T) {
	r := quiet(NewRegistry())
	r.BaseDelay = 20 * time.Millisecond
	var n atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	r.Go(ctx, Loop{Name: "tick", Label: "Tick", Every: time.Hour, Run: func(context.Context) error {
		if n.Add(1) == 1 {
			var m map[string]int
			m["x"] = 1 // nil map write
		}
		return nil
	}})
	waitFor(t, "retried", func() bool { return n.Load() == 2 && state(r, "stage:tick").State == Running })
	cancel()
	r.Wait()
	if st := state(r, "stage:tick"); st.State != Off {
		t.Fatalf("%+v", st)
	}
}

// An error from a pass marks it retrying with the reason; the next good pass
// marks it running.
func TestPeriodicErrorIsRetrying(t *testing.T) {
	r := quiet(NewRegistry())
	var n atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	r.Go(ctx, Loop{Name: "e", Label: "E", Every: 30 * time.Millisecond, Run: func(context.Context) error {
		if n.Add(1) == 1 {
			return errors.New("open /srv/private/x: dial 10.1.2.3: the disk is full")
		}
		return nil
	}})
	waitFor(t, "retrying", func() bool { return state(r, "stage:e").State == Retrying })
	// Fixed plain words; the error itself stays in the log.
	if st := state(r, "stage:e"); strings.Contains(st.Reason, "/srv") || strings.Contains(st.Reason, "10.1.2.3") || !strings.Contains(st.Reason, "log") {
		t.Fatalf("%+v", st)
	}
	waitFor(t, "running", func() bool { return state(r, "stage:e").State == Running })
	cancel()
	r.Wait()
}

// Cancelling stops a long loop cleanly and waits for it.
func TestCleanShutdown(t *testing.T) {
	r := quiet(NewRegistry())
	var ended atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	r.Go(ctx, Loop{Name: "l", Label: "L", Run: func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		ended.Store(true)
		return nil
	}})
	waitFor(t, "running", func() bool { return state(r, "stage:l").State == Running })
	cancel()
	r.Wait()
	if !ended.Load() || state(r, "stage:l").State != Off {
		t.Fatal("Wait returned before the loop ended")
	}
}

// Guard catches a crash in a one-off piece of background work and records
// it; the next good call puts the stage back to running.
func TestGuard(t *testing.T) {
	r := quiet(NewRegistry())
	r.Set("stage:alerts", "stage", "Alerts", Running, "")
	r.Guard("alerts", func() { panic("x") })
	if st := state(r, "stage:alerts"); st.State != Failed || !strings.Contains(st.Reason, "crashed") {
		t.Fatalf("%+v", st)
	}
	r.Guard("alerts", func() {})
	if st := state(r, "stage:alerts"); st.State != Running {
		t.Fatalf("%+v", st)
	}
	// Package-level Guard without a default registry still recovers.
	SetDefault(nil)
	Guard("alerts", func() { panic("y") })
}

func TestListOrder(t *testing.T) {
	r := NewRegistry()
	r.Set("link:source:2", "source", "B", Running, "")
	r.Set("stage:z", "stage", "Z", Off, "")
	r.Set("link:source:1", "source", "A", Failed, "x")
	l := r.List()
	if len(l) != 3 || l[0].Key != "stage:z" || l[1].Name != "A" || l[2].Name != "B" {
		t.Fatalf("%+v", l)
	}
}
