package fetchq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func open(t *testing.T, path string, c *clock, do func(context.Context, string, int) error) *Queue {
	t.Helper()
	q, err := Open(path, Options{Name: "test", Rate: 0.9, Do: do, Now: c.now, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	return q
}

func TestWaitingJumpsAheadAndDedupes(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(_ context.Context, k string, p int) error { ran = append(ran, fmt.Sprint(k, p)); return nil })
	ctx := context.Background()
	for i := range 5 {
		q.Enqueue(ctx, fmt.Sprint("list", i), P2Shared, false)
	}
	q.Enqueue(ctx, "list3", P4Deep, false) // lower: keeps P2
	c.t = c.t.Add(time.Second)
	done := q.Enqueue(ctx, "user", P0Waiting, true)
	c.t = c.t.Add(time.Second)
	q.Enqueue(ctx, "list4", P0Waiting, false) // raised to P0
	q.Step(ctx)
	if ran[0] != "user0" {
		t.Fatalf("P0 did not go first: %v", ran)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	q.Step(ctx)
	if ran[1] != "list40" {
		t.Fatalf("raised job not next: %v", ran)
	}
	for q.Step(ctx) {
	}
	if len(ran) != 6 {
		t.Fatalf("duplicates or losses: %v", ran)
	}
}

func TestSharesOverASimulatedHour(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	count := [5]int{}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(_ context.Context, _ string, p int) error { count[p]++; return nil })
	ctx := context.Background()
	n := 0
	added := [5]int{}
	refill := func() {
		for p := range 5 {
			for added[p]-count[p] < 2 {
				n++
				added[p]++
				q.Enqueue(ctx, fmt.Sprint(p, ":", n), p, false)
			}
		}
	}
	step := 1111 * time.Millisecond
	total := 0
	for c.t.Before(time.Unix(1_800_000_000, 0).Add(time.Hour)) {
		refill()
		if q.Step(ctx) {
			total++
		}
		c.t = c.t.Add(step)
	}
	// Someone always waits here: P0 takes its share, P1 (pages just shown)
	// the rest, and the server's own caching work (P2 to P4) waits.
	want := [5]float64{0.60, 0.40, 0, 0, 0}
	for p := range 5 {
		got := float64(count[p]) / float64(total)
		if math.Abs(got-want[p]) > 0.05 {
			t.Fatalf("P%d share %.3f want %.2f (%v of %d)", p, got, want[p], count, total)
		}
	}
	if total < 3200 {
		t.Fatalf("only %d jobs in an hour at 0.9/s", total)
	}
}

func TestThrottleHalvesRate(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	first := true
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error {
		if first {
			first = false
			return ErrThrottled
		}
		return nil
	})
	ctx := context.Background()
	q.Enqueue(ctx, "a", P2Shared, false)
	q.Step(ctx)
	if r := q.Stats(ctx).Rate; r != 0.45 {
		t.Fatalf("rate after 503 %v", r)
	}
	if q.Stats(ctx).Queued[P2Shared] != 1 {
		t.Fatal("throttled job lost")
	}
	c.t = c.t.Add(61 * time.Second)
	if r := q.Stats(ctx).Rate; r <= 0.45 || r >= 0.9 {
		t.Fatalf("rate did not step up %v", r)
	}
	c.t = c.t.Add(10 * time.Minute)
	if r := q.Stats(ctx).Rate; r != 0.9 {
		t.Fatalf("rate not restored %v", r)
	}
}

func TestRestartRequeuesRunningWithoutDuplicates(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	path := filepath.Join(t.TempDir(), "q.sqlite")
	q, err := Open(path, Options{Do: func(context.Context, string, int) error { return nil }, Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	q.Enqueue(ctx, "a", P2Shared, false)
	q.Enqueue(ctx, "b", P2Shared, false)
	q.db.Exec(`UPDATE fetch_jobs SET state='running' WHERE key='a'`)
	q.Close()
	var ran []string
	q2 := open(t, path, c, func(_ context.Context, k string, _ int) error { ran = append(ran, k); return nil })
	q2.Enqueue(ctx, "a", P2Shared, false)
	for q2.Step(ctx) {
	}
	if len(ran) != 2 {
		t.Fatalf("after restart ran %v", ran)
	}
}

func TestPanicCountsAsFailure(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error {
		var m map[string]int
		m["x"] = 1 // nil map write panics
		return nil
	})
	ctx := context.Background()
	q.Enqueue(ctx, "boom", P2Shared, false)
	for i := range 5 {
		if !q.Step(ctx) {
			t.Fatalf("step %d did not run", i)
		}
		c.t = c.t.Add(7 * time.Hour)
	}
	if st := q.Stats(ctx); st.Dead != 1 {
		t.Fatalf("panicking job not dead: %+v", st)
	}
}

func TestDeadAfterFiveFailuresAndNotFound(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(_ context.Context, k string, _ int) error {
		if k == "gone" {
			return ErrNotFound
		}
		return errors.New("upstream 500")
	})
	ctx := context.Background()
	q.Enqueue(ctx, "bad", P2Shared, false)
	for i := range 5 {
		if !q.Step(ctx) {
			t.Fatalf("step %d did not run", i)
		}
		c.t = c.t.Add(7 * time.Hour)
	}
	if st := q.Stats(ctx); st.Dead != 1 || st.Queued[P2Shared] != 0 {
		t.Fatalf("not dead after 5 failures: %+v", st)
	}
	if ch := q.Enqueue(ctx, "bad", P2Shared, true); <-ch != ErrDead {
		t.Fatal("dead key re-queued by P2 within 24 h")
	}
	if ch := q.Enqueue(ctx, "bad", P0Waiting, false); ch != nil || q.Stats(ctx).Queued[P0Waiting] != 1 {
		t.Fatal("P0 did not revive a dead key")
	}
	w := q.Enqueue(ctx, "gone", P0Waiting, true)
	q.Step(ctx) // bad (P0, older)
	q.Step(ctx)
	if err := <-w; !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 waiter got %v", err)
	}
}

func TestLaneSwitchPausesAndCapDrops(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	on := false
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{MaxRows: 2, Now: c.now, Do: func(context.Context, string, int) error { return nil }, Lane: func(p int) bool { return p != P2Shared || on }})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	q.Enqueue(ctx, "a", P2Shared, false)
	q.Enqueue(ctx, "b", P2Shared, false)
	q.Enqueue(ctx, "c", P4Deep, false)
	if st := q.Stats(ctx); st.Dropped != 1 {
		t.Fatalf("over cap P4 not dropped %+v", st)
	}
	if q.Step(ctx) {
		t.Fatal("paused lane ran")
	}
	on = true
	if !q.Step(ctx) {
		t.Fatal("lane did not resume")
	}
}

func TestReserveLeavesShareForMatching(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	share := 0.45
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "test", Rate: 0.9, Do: func(context.Context, string, int) error { return nil }, Now: c.now, Logf: func(string, ...any) {},
		Reserve: func() float64 { return share }})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if r := q.Stats(context.Background()).Rate; r < 0.49 || r > 0.5 {
		t.Fatalf("rate with reserve %v", r)
	}
	share = 3 // capped at half
	if r := q.Stats(context.Background()).Rate; r != 0.45 {
		t.Fatalf("rate with big reserve %v", r)
	}
	share = 0
	if r := q.Stats(context.Background()).Rate; r != 0.9 {
		t.Fatalf("rate without reserve %v", r)
	}
}

func TestStateAnswersQueuedAndDead(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return ErrNotFound })
	ctx := context.Background()
	if st, _ := q.State(ctx, "x"); st != "" {
		t.Fatalf("unknown job has state %q", st)
	}
	q.Enqueue(ctx, "x", P4Deep, false)
	if st, _ := q.State(ctx, "x"); st != "queued" {
		t.Fatalf("queued job has state %q", st)
	}
	q.Step(ctx)
	if st, at := q.State(ctx, "x"); st != "dead" || !at.Equal(c.t) {
		t.Fatalf("dead job: %q %v", st, at)
	}
}

// Someone waiting (P0) always goes before the server's own caching work,
// even past the P0 share; the lanes below wait.
func TestWaitingAlwaysBeforeCaching(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "t", Rate: 1, Do: func(_ context.Context, k string, _ int) error { ran = append(ran, k); return nil }, Now: c.now, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	for i := range 3 {
		q.Enqueue(ctx, fmt.Sprint("deep", i), P4Deep, false)
		q.Enqueue(ctx, fmt.Sprint("shared", i), P2Shared, false)
	}
	for i := range 70 { // more than the P0 share of a minute
		q.Enqueue(ctx, fmt.Sprint("wait", i), P0Waiting, false)
	}
	for range 70 {
		if !q.Step(ctx) {
			c.t = c.t.Add(time.Second)
			continue
		}
	}
	for _, k := range ran {
		if k[:4] != "wait" {
			t.Fatalf("caching job %s ran while someone waited: %v", k, ran)
		}
	}
}

// P0 jobs from different installs are served in turn: one install's
// burst never pushes another's single ask to the back.
func TestWaitingFairAcrossInstalls(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(_ context.Context, k string, _ int) error { ran = append(ran, k); return nil })
	ctx := context.Background()
	a, b := WithInstall(ctx, "install-a"), WithInstall(ctx, "install-b")
	for i := range 20 {
		q.Enqueue(a, fmt.Sprint("a", i), P0Waiting, false)
	}
	c.t = c.t.Add(time.Millisecond)
	q.Enqueue(b, "b0", P0Waiting, false)
	q.Enqueue(b, "b1", P0Waiting, false)
	for range 6 {
		q.Step(ctx)
	}
	pos := map[string]int{}
	for i, k := range ran {
		pos[k] = i
	}
	if p, ok := pos["b0"]; !ok || p > 1 {
		t.Fatalf("install b's first ask waited behind a's burst: %v", ran)
	}
	if p, ok := pos["b1"]; !ok || p > 3 {
		t.Fatalf("install b's second ask not served in turn: %v", ran)
	}
	if ran[0] != "a0" {
		t.Fatalf("first come is not served first: %v", ran)
	}
}

// One install with 1,000 queued asks never starves another: the second
// install's ask runs next, and the first's asks past 200 wait at P2.
func TestBigInstallCannotStarveAnother(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(_ context.Context, k string, _ int) error { ran = append(ran, k); return nil })
	ctx := context.Background()
	a := WithInstall(ctx, "big")
	for i := range 1000 {
		q.Enqueue(a, fmt.Sprint("big", i), P0Waiting, false)
	}
	if st := q.Stats(ctx); st.Queued[P0Waiting] != p0PerInstall || st.Queued[P2Shared] != 1000-p0PerInstall {
		t.Fatalf("per-install cap: queued %v", st.Queued)
	}
	c.t = c.t.Add(time.Second)
	q.Enqueue(WithInstall(ctx, "small"), "small0", P0Waiting, false)
	q.Step(ctx)
	q.Step(ctx)
	if len(ran) != 2 || ran[0] != "big0" || ran[1] != "small0" {
		t.Fatalf("the second install waited behind the big one: %v", ran)
	}
}

// 20,000 rotating install IDs never reset an existing install's history.
func TestRotatingInstallsKeepOthersHistory(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return nil })
	q.mu.Lock()
	defer q.mu.Unlock()
	for range 5 {
		q.chargeLocked("steady", c.t)
	}
	for i := range 20000 {
		if i%1000 == 0 {
			c.t = c.t.Add(time.Second)
		}
		q.chargeLocked(fmt.Sprint("rot", i), c.t)
	}
	c.t = c.t.Add(30 * time.Second)
	q.chargeLocked("x", c.t)
	if n := q.ranLocked("steady", c.t); n != 5 {
		t.Fatalf("steady install's history %d after rotating IDs, want 5", n)
	}
	c.t = c.t.Add(2 * time.Minute)
	q.chargeLocked("y", c.t)
	if len(q.byInstall) > 2 {
		t.Fatalf("%d installs kept after the window, want only the recent ones", len(q.byInstall))
	}
}

// An install at its P0 cap cannot bring dead jobs back at P0: a revive goes
// through the same cap (P2) and counts against the install that asked.
func TestReviveAtCapGetsP2(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return ErrNotFound })
	ctx := context.Background()
	for i := range 3 {
		q.Enqueue(ctx, fmt.Sprint("old", i), P4Deep, false)
	}
	for q.Step(ctx) {
	}
	if st := q.Stats(ctx); st.Dead != 3 {
		t.Fatalf("dead %d", st.Dead)
	}
	a := WithInstall(ctx, "busy")
	for i := range p0PerInstall {
		q.Enqueue(a, fmt.Sprint("ask", i), P0Waiting, false)
	}
	for i := range 3 {
		q.Enqueue(a, fmt.Sprint("old", i), P0Waiting, false)
	}
	st := q.Stats(ctx)
	if st.Queued[P0Waiting] != p0PerInstall || st.Queued[P2Shared] != 3 {
		t.Fatalf("revived at the cap: queued %v", st.Queued)
	}
	if s, _ := q.State(ctx, "old0"); s != "queued" {
		t.Fatalf("old0 %s", s)
	}
}

// A job waiting on another (ErrLater) is queued again later and never dies
// of it, however often it waits.
func TestLaterNeverDies(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	n := 0
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error {
		n++
		if n < LaterMax {
			return ErrLater
		}
		return nil
	})
	ctx := context.Background()
	q.Enqueue(ctx, "pic", P2Shared, false)
	for range 10 {
		q.Step(ctx)
		if st, _ := q.State(ctx, "pic"); st == "dead" {
			t.Fatal("a waiting job died")
		}
		c.t = c.t.Add(LaterWait)
	}
	if st, _ := q.State(ctx, "pic"); st != "" || n != 10 {
		t.Fatalf("state %q after %d runs", st, n)
	}
}

// A job that only ever waits (its input can never come) stops: past
// LaterMax waits each further wait is an ordinary failure, it backs off and
// ends dead, and is no longer picked.
func TestEndlessWaitEndsDead(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	n := 0
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { n++; return ErrLater })
	ctx := context.Background()
	q.Enqueue(ctx, "pic", P2Shared, false)
	for range LaterMax + len(Backoff) + 5 {
		q.Step(ctx)
		c.t = c.t.Add(7 * time.Hour)
	}
	if st, _ := q.State(ctx, "pic"); st != "dead" {
		t.Fatalf("endless waiter is %q", st)
	}
	if n != LaterMax+len(Backoff) {
		t.Fatalf("ran %d times, want %d", n, LaterMax+len(Backoff))
	}
}

// With Workers > 1 jobs run side by side, but one slot is kept for P0 and
// P1: long P2 jobs never fill every worker, so a waiting user's job starts
// at once.
func TestWorkersKeepASlotForWaitingJobs(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	running := map[string]bool{}
	started := make(chan string, 16)
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "t", Rate: 1000, Workers: 3, Logf: func(string, ...any) {},
		Do: func(_ context.Context, k string, _ int) error {
			mu.Lock()
			running[k] = true
			mu.Unlock()
			started <- k
			if k[:4] == "slow" {
				<-release
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := range 5 {
		q.Enqueue(ctx, fmt.Sprint("slow", i), P2Shared, false)
	}
	go q.Run(ctx)
	for range 2 {
		<-started
	}
	select {
	case k := <-started:
		t.Fatalf("a third P2 job %s took the slot kept for waiting jobs", k)
	case <-time.After(300 * time.Millisecond):
	}
	q.Enqueue(ctx, "user", P0Waiting, false)
	select {
	case k := <-started:
		if k != "user" {
			t.Fatalf("got %s, want the waiting job", k)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the waiting job did not start")
	}
	close(release)
}

// In the P2 lane heavy jobs (artist picture chains) alternate with light
// ones (album covers): one heavy after every HeavyEvery light, so neither
// kind waits behind a long run of the other.
func TestHeavyJobsAlternateWithLight(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "t", Rate: 1, Now: c.now, Logf: func(string, ...any) {},
		HeavyPrefixes: []string{"img:artist:"}, HeavyEvery: 3,
		Do: func(_ context.Context, k string, _ int) error { ran = append(ran, k[:7]); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	for i := range 6 {
		q.Enqueue(ctx, fmt.Sprint("img:artist:", i), P2Shared, false) // queued first
	}
	c.t = c.t.Add(time.Second)
	for i := range 6 {
		q.Enqueue(ctx, fmt.Sprint("img:album:", i), P2Shared, false)
	}
	for range 8 {
		q.Step(ctx)
		c.t = c.t.Add(time.Second)
	}
	want := "img:alb,img:alb,img:alb,img:art,img:alb,img:alb,img:alb,img:art"
	if got := strings.Join(ran, ","); got != want {
		t.Fatalf("order %s\nwant  %s", got, want)
	}
}

// A walk steps one call a run (ErrMore): a waiting user's job runs between
// its steps, its waiter hears only the last step, and a waiter that joins
// mid-walk lifts the rest of the walk to P0.
func TestMoreStepsYieldAndNotifyAtEnd(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	steps := map[string]int{}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(_ context.Context, k string, p int) error {
		ran = append(ran, fmt.Sprint(k, p))
		steps[k]++
		if k == "walk" && steps[k] < 3 {
			return ErrMore
		}
		return nil
	})
	ctx := context.Background()
	q.Enqueue(ctx, "walk", P4Deep, false)
	if !q.Step(ctx) {
		t.Fatal("no step")
	}
	done := q.Enqueue(ctx, "walk", P0Waiting, true) // queued: lifted at once
	user := q.Enqueue(ctx, "user", P0Waiting, true)
	for q.Step(ctx) {
		c.t = c.t.Add(time.Second)
	}
	if strings.Join(ran, ",") != "walk4,walk0,user0,walk0" {
		t.Fatalf("order %v", ran)
	}
	for _, ch := range []<-chan error{done, user} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("waiter not told")
		}
	}
	if st := q.Stats(ctx); st.Failed != 0 || st.Done != 2 {
		t.Fatalf("stats %+v", st)
	}
}

// A job that cannot connect is queued again without counting a failure,
// however often (here 40 times, far past Backoff's five), and the queue
// with ConnPause waits as a whole; the job then finishes once the host
// answers.
func TestConnectionFailuresNeverCount(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	n := 0
	var logs []string
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "test", Rate: 0.9, Now: c.now, ConnPause: true,
		Logf: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) },
		Do: func(context.Context, string, int) error {
			n++
			if n <= 40 {
				return &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	q.Enqueue(ctx, "mb:artist:x", P2Shared, false)
	q.Enqueue(ctx, "mb:artist:y", P2Shared, false)
	if !q.Step(ctx) {
		t.Fatal("did not run")
	}
	// Paused: nothing runs, even a ready job.
	if q.Step(ctx) {
		t.Fatal("ran while paused after a connection failure")
	}
	st := q.Stats(ctx)
	if st.PausedUntil.IsZero() || st.ConnRun != 1 || st.Failed != 0 {
		t.Fatalf("stats after one failure: %+v", st)
	}
	for range 200 {
		c.t = c.t.Add(5 * time.Minute)
		for q.Step(ctx) {
		}
		if s, _ := q.State(ctx, "mb:artist:x"); s == "dead" {
			t.Fatal("job died of connection failures")
		}
	}
	st = q.Stats(ctx)
	if s, _ := q.State(ctx, "mb:artist:x"); s != "" || st.Failed != 0 || st.Dead != 0 || st.ConnRetries != 40 || st.Done != 2 {
		t.Fatalf("state %q stats %+v", s, st)
	}
	alarm := false
	for _, l := range logs {
		alarm = alarm || strings.Contains(l, "ALARM")
	}
	if !alarm {
		t.Fatalf("no alarm logged: %v", logs)
	}
}

// ErrConnection and wrapped transport errors count the same; an answer
// such as a 500 still counts.
func TestConnErrorKinds(t *testing.T) {
	for _, err := range []error{ErrConnection, fmt.Errorf("get: %w", io.ErrUnexpectedEOF), errors.New("remote error: tls: handshake failure"), &net.DNSError{Err: "no such host", Name: "musicbrainz.org"}} {
		if !connError(err) {
			t.Errorf("%v not a connection failure", err)
		}
	}
	for _, err := range []error{errors.New("status 500"), ErrNotFound, ErrLater, ErrThrottled} {
		if connError(err) {
			t.Errorf("%v taken for a connection failure", err)
		}
	}
}

// A job waiting on another (WaitOn) never dies while that job is queued,
// and lifts it to its own priority; once the input is gone the waits count
// again.
func TestWaitOnInputQueued(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	mb := open(t, filepath.Join(t.TempDir(), "mb.sqlite"), c, func(context.Context, string, int) error { return nil })
	var imgq *Queue
	imgq = open(t, filepath.Join(t.TempDir(), "img.sqlite"), c, func(context.Context, string, int) error { return mb.WaitOn("mb:artist:a") })
	ctx := context.Background()
	mb.Enqueue(ctx, "mb:artist:a", P4Deep, false)
	imgq.Enqueue(ctx, "img:artist:a", P2Shared, false)
	for range LaterMax * 3 {
		imgq.Step(ctx)
		c.t = c.t.Add(LaterWait)
	}
	if s, _ := imgq.State(ctx, "img:artist:a"); s != "queued" {
		t.Fatalf("waiter is %q while its input is queued", s)
	}
	var p int
	if err := mb.db.QueryRow(`SELECT priority FROM fetch_jobs WHERE key='mb:artist:a'`).Scan(&p); err != nil || p != P2Shared {
		t.Fatalf("input priority %d (%v), want raised to P2", p, err)
	}
	if st := imgq.Stats(ctx); st.Waits != int64(LaterMax*3) || st.Failed != 0 {
		t.Fatalf("stats %+v", st)
	}
	// The input is done (and the waiter still waits): waits count again.
	for mb.Step(ctx) {
	}
	for range LaterMax + len(Backoff) + 2 {
		imgq.Step(ctx)
		c.t = c.t.Add(7 * time.Hour)
	}
	if s, _ := imgq.State(ctx, "img:artist:a"); s != "dead" {
		t.Fatalf("endless waiter with no input is %q", s)
	}
}

// Hold puts a job back for the given time without running or counting it.
func TestHoldPutsBack(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	var ran []string
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Name: "test", Rate: 0.9, Now: c.now, Logf: func(string, ...any) {},
		Do: func(_ context.Context, k string, _ int) error { ran = append(ran, k); return nil },
		Hold: func(k string, p int) (bool, time.Duration) {
			return strings.HasPrefix(k, "held:"), time.Hour
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	q.Enqueue(ctx, "held:1", P2Shared, false)
	q.Enqueue(ctx, "free:1", P3Refresh, false)
	for q.Step(ctx) {
	}
	if len(ran) != 1 || ran[0] != "free:1" {
		t.Fatalf("ran %v", ran)
	}
	if s, nb := q.State(ctx, "held:1"); s != "queued" || nb.Sub(c.t) != time.Hour {
		t.Fatalf("held job %q until %v", s, nb)
	}
	if st := q.Stats(ctx); st.Held != 1 || st.Failed != 0 {
		t.Fatalf("stats %+v", st)
	}
}

// A full shared pace (upstream.ErrBusy) sends nothing: the job waits,
// uncounted, and never dies of it.
func TestPaceBusyNeverCounts(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	n := 0
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error {
		n++
		if n <= 20 {
			return fmt.Errorf("get: %w", upstream.ErrBusy)
		}
		return nil
	})
	ctx := context.Background()
	q.Enqueue(ctx, "k", P2Shared, false)
	for range 30 {
		q.Step(ctx)
		c.t = c.t.Add(time.Minute)
	}
	if st := q.Stats(ctx); st.Failed != 0 || st.Done != 1 || st.ConnRun != 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestPositionCountsJobsAhead(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return nil })
	ctx := context.Background()
	if st, n := q.Position(ctx, "none"); st != "" || n != 0 {
		t.Fatalf("missing job: %q %d", st, n)
	}
	for i := range 3 {
		q.Enqueue(ctx, fmt.Sprint("deep", i), P4Deep, false)
		c.t = c.t.Add(time.Millisecond)
	}
	q.Enqueue(ctx, "a", P0Waiting, false)
	c.t = c.t.Add(time.Millisecond)
	q.Enqueue(ctx, "b", P0Waiting, false)
	for key, want := range map[string]int{"a": 1, "b": 2, "deep0": 3, "deep2": 5} {
		if st, n := q.Position(ctx, key); st != "queued" || n != want {
			t.Fatalf("%s: %q %d, want queued %d", key, st, n, want)
		}
	}
	q.Step(ctx) // runs a
	if st, n := q.Position(ctx, "b"); st != "queued" || n != 1 {
		t.Fatalf("b after a ran: %q %d", st, n)
	}
}

// L1: the MaxRows check counts the jobs at most every rowsEvery; inserts
// in between are added to the kept count, so the cap still holds.
func TestCapCountKeptBetweenCounts(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{MaxRows: 3, Now: c.now, Do: func(context.Context, string, int) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c", "d"} {
		q.Enqueue(ctx, k, P3Refresh, false)
	}
	if st := q.Stats(ctx); st.Dropped != 1 {
		t.Fatalf("cap not held: %+v", st)
	}
	// Jobs removed behind the queue's back are seen at the next count.
	_, _ = q.db.Exec(`DELETE FROM fetch_jobs`)
	q.Enqueue(ctx, "e", P3Refresh, false)
	if st := q.Stats(ctx); st.Dropped != 2 {
		t.Fatalf("recounted too soon: %+v", st)
	}
	c.t = c.t.Add(rowsEvery + time.Second)
	q.Enqueue(ctx, "f", P3Refresh, false)
	if st := q.Stats(ctx); st.Dropped != 2 {
		t.Fatalf("not recounted after rowsEvery: %+v", st)
	}
}

// L2: two claimers at once never run the same job twice.
func TestClaimRunsEachJobOnce(t *testing.T) {
	var mu sync.Mutex
	runs := map[string]int{}
	q, err := Open(filepath.Join(t.TempDir(), "q.sqlite"), Options{Do: func(_ context.Context, key string, _ int) error {
		mu.Lock()
		runs[key]++
		mu.Unlock()
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		q.Enqueue(ctx, fmt.Sprint("k", i), P2Shared, false)
	}
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				q.Step(ctx)
			}
		}()
	}
	wg.Wait()
	for k, n := range runs {
		if n != 1 {
			t.Fatalf("%s ran %d times", k, n)
		}
	}
}

// A waiter whose input died is released at once (dead too: its input will
// not come), and one that has waited past WaitMax on an input still queued
// stops waiting uncounted (audit 2026-10-29: hv: jobs stuck since 10-26).
func TestWaitOnDeadOrStaleInput(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	mb := open(t, filepath.Join(t.TempDir(), "mb.sqlite"), c, func(context.Context, string, int) error { return ErrNotFound })
	imgq := open(t, filepath.Join(t.TempDir(), "img.sqlite"), c, func(_ context.Context, k string, _ int) error {
		return mb.WaitOn("mb:" + k)
	})
	ctx := context.Background()
	mb.Enqueue(ctx, "mb:dies", P3Refresh, false)
	mb.Enqueue(ctx, "mb:stuck", P4Deep, false)
	imgq.Enqueue(ctx, "dies", P3Refresh, false)
	imgq.Enqueue(ctx, "stuck", P3Refresh, false)
	// The first input dies.
	mb.Step(ctx)
	if s, _ := mb.State(ctx, "mb:dies"); s != "dead" {
		t.Fatalf("input %q", s)
	}
	for imgq.Step(ctx) {
	}
	if s, _ := imgq.State(ctx, "dies"); s != "dead" {
		t.Fatalf("waiter on a dead input is %q", s)
	}
	if s, _ := imgq.State(ctx, "stuck"); s != "queued" {
		t.Fatalf("young waiter is %q", s)
	}
	// Past WaitMax the stuck waiter's waits count, so it ends.
	c.t = c.t.Add(WaitMax + time.Minute)
	for range LaterMax + len(Backoff) + 2 {
		imgq.Step(ctx)
		c.t = c.t.Add(7 * time.Hour)
	}
	if s, _ := imgq.State(ctx, "stuck"); s != "dead" {
		t.Fatalf("stale waiter is %q", s)
	}
}

// Lower moves queued jobs of a prefix down to a background priority and
// leaves higher-numbered, running and other jobs alone.
func TestLowerPrefix(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return nil })
	ctx := context.Background()
	q.Enqueue(ctx, "big:album:a", P3Refresh, false)
	q.Enqueue(ctx, "big:album:b", P1Finish, false)
	q.Enqueue(ctx, "img:album:a", P3Refresh, false)
	if n := q.Lower(ctx, "big:", P4Deep, P3Refresh); n != 1 {
		t.Fatalf("lowered %d, want 1", n)
	}
	prio := func(k string) int {
		var p int
		_ = q.db.QueryRow(`SELECT priority FROM fetch_jobs WHERE key=?`, k).Scan(&p)
		return p
	}
	if prio("big:album:a") != P4Deep || prio("big:album:b") != P1Finish || prio("img:album:a") != P3Refresh {
		t.Fatal("wrong rows lowered")
	}
}

// Revive puts a dead job back once it has been dead ReviveAfter (not at
// once, so a lasting failure is not asked every sweep); a queued job only
// rises, and a missing one is made.
func TestRevive(t *testing.T) {
	c := &clock{time.Unix(1_800_000_000, 0)}
	q := open(t, filepath.Join(t.TempDir(), "q.sqlite"), c, func(context.Context, string, int) error { return ErrNotFound })
	ctx := context.Background()
	q.Enqueue(ctx, "img:artist:a", P3Refresh, false)
	q.Step(ctx)
	if s, _ := q.State(ctx, "img:artist:a"); s != "dead" {
		t.Fatalf("state %q", s)
	}
	q.Enqueue(ctx, "img:artist:a", P2Shared, false) // dead within 24 h: refused
	if s, _ := q.State(ctx, "img:artist:a"); s != "dead" {
		t.Fatal("plain enqueue revived a fresh dead job")
	}
	q.Revive(ctx, "img:artist:a", P2Shared)
	if s, _ := q.State(ctx, "img:artist:a"); s != "dead" {
		t.Fatal("revived a job that just died")
	}
	c.t = c.t.Add(ReviveAfter)
	q.Revive(ctx, "img:artist:a", P2Shared)
	q.Revive(ctx, "img:artist:b", P2Shared)
	for _, k := range []string{"img:artist:a", "img:artist:b"} {
		if s, _ := q.State(ctx, k); s != "queued" {
			t.Fatalf("%s %q", k, s)
		}
	}
}
