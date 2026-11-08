package jobs

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

var me = database.Actor{Kind: "user", ID: 1}

func newStore(t *testing.T, dir string) *database.Store {
	t.Helper()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fastOpts() Options {
	return Options{Workers: 2, BaseBackoff: 5 * time.Millisecond, PollInterval: 5 * time.Millisecond}
}

func getJob(t *testing.T, s *database.Store, id int64) Job {
	t.Helper()
	var j Job
	if err := database.Get(context.Background(), s.Read(), "jobs", id, &j); err != nil {
		t.Fatal(err)
	}
	return j
}

func waitState(t *testing.T, s *database.Store, id int64, want string) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j := getJob(t, s, id); j.State == want {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	j := getJob(t, s, id)
	t.Fatalf("job %d: state %s, want %s", id, j.State, want)
	return j
}

func TestDedupe(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	ctx := context.Background()
	a, created, err := q.Enqueue(ctx, "search_slot", &database.Subject{Type: "slot", ID: 1}, nil, "search_slot:1", me)
	if err != nil || !created {
		t.Fatalf("first enqueue: %v %v", created, err)
	}
	b, created, err := q.Enqueue(ctx, "search_slot", &database.Subject{Type: "slot", ID: 1}, nil, "search_slot:1", me)
	if err != nil || created || b != a {
		t.Fatalf("second enqueue should return job %d unchanged, got %d %v %v", a, b, created, err)
	}
	c, created, _ := q.Enqueue(ctx, "search_slot", nil, nil, "search_slot:2", me)
	if !created || c == a {
		t.Fatal("a different key should make a new job")
	}
	// Once the first job is finished the key is free again.
	q.Handle("search_slot", func(context.Context, *Run) error { return nil })
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	waitState(t, s, a, "done")
	d, created, _ := q.Enqueue(ctx, "search_slot", nil, nil, "search_slot:1", me)
	if !created || d == a {
		t.Fatal("key should be free after the job is done")
	}
}

func TestRetryWithBackoff(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	var calls atomic.Int32
	var times []time.Time
	var mu sync.Mutex
	q.Handle("flaky", func(context.Context, *Run) error {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		if calls.Add(1) < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	ctx := context.Background()
	id, _, _ := q.Enqueue(ctx, "flaky", nil, nil, "", me)
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	j := waitState(t, s, id, "done")
	if j.Attempt != 3 || calls.Load() != 3 {
		t.Fatalf("attempts %d, calls %d, want 3", j.Attempt, calls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if gap1, gap2 := times[1].Sub(times[0]), times[2].Sub(times[1]); gap1 < 5*time.Millisecond || gap2 < 10*time.Millisecond {
		t.Fatalf("back-off too short: %v then %v", gap1, gap2)
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	o := fastOpts()
	o.MaxAttempts = 2
	q := New(s, o)
	q.Handle("broken", func(context.Context, *Run) error { return errors.New("always") })
	ctx := context.Background()
	id, _, _ := q.Enqueue(ctx, "broken", nil, nil, "", me)
	_ = q.Start(ctx)
	defer q.Stop()
	j := waitState(t, s, id, "failed")
	if j.Attempt != 2 || j.Error == nil || j.Error.Message != "always" || j.EndedAt == nil {
		t.Fatalf("failed job wrong: %+v", j)
	}
}

func TestUnknownKindFails(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	o := fastOpts()
	o.MaxAttempts = 1
	q := New(s, o)
	ctx := context.Background()
	id, _, _ := q.Enqueue(ctx, "nobody_handles_this", nil, nil, "", me)
	_ = q.Start(ctx)
	defer q.Stop()
	j := waitState(t, s, id, "failed")
	if j.Error == nil || j.Error.Code != "no_handler" {
		t.Fatalf("want no_handler, got %+v", j.Error)
	}
}

func TestCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	q := New(s, fastOpts())
	ctx := context.Background()
	id, _, _ := q.Enqueue(ctx, "scan_folder", nil, nil, "scan:1", me)
	// Simulate a crash mid-job: the row says running, the process is gone.
	err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET state = 'running', attempt = 1, started_at = ? WHERE id = ?", database.FormatTime(time.Now()), id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	s = newStore(t, dir)
	defer s.Close()
	q = New(s, fastOpts())
	var ran atomic.Bool
	q.Handle("scan_folder", func(context.Context, *Run) error { ran.Store(true); return nil })
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	j := waitState(t, s, id, "done")
	if !ran.Load() || j.Attempt != 2 {
		t.Fatalf("recovered job: ran %v attempt %d", ran.Load(), j.Attempt)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	o := fastOpts()
	o.Workers = 3
	q := New(s, o)
	var now, peak atomic.Int32
	q.Handle("work", func(context.Context, *Run) error {
		n := now.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		now.Add(-1)
		return nil
	})
	ctx := context.Background()
	var ids []int64
	for range 9 {
		id, _, _ := q.Enqueue(ctx, "work", nil, nil, "", me)
		ids = append(ids, id)
	}
	_ = q.Start(ctx)
	defer q.Stop()
	for _, id := range ids {
		waitState(t, s, id, "done")
	}
	if peak.Load() != 3 {
		t.Fatalf("peak concurrency %d, want 3", peak.Load())
	}
}

func TestDefaultWorkers(t *testing.T) {
	if New(nil, Options{}).opt.Workers != 2 {
		t.Fatal("default worker count should be 2")
	}
}

func TestProgressAndResult(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	seen := make(chan float64, 10)
	q.opt.OnChange = func(j Job) {
		if j.State == "running" && j.Progress > 0 {
			seen <- j.Progress
		}
	}
	q.Handle("steps", func(ctx context.Context, r *Run) error {
		if err := r.Progress(ctx, 0.5, "half way"); err != nil {
			return err
		}
		r.SetResult(map[string]any{"found": 3.0})
		return nil
	})
	ctx := context.Background()
	id, _, _ := q.Enqueue(ctx, "steps", nil, map[string]any{"a": 1.0}, "", me)
	_ = q.Start(ctx)
	defer q.Stop()
	j := waitState(t, s, id, "done")
	if j.Progress != 1 || j.Result["found"] != 3.0 || j.ProgressNote == nil || *j.ProgressNote != "half way" {
		t.Fatalf("job after run: %+v", j)
	}
	select {
	case p := <-seen:
		if p != 0.5 {
			t.Fatalf("progress %v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no progress change seen")
	}
}

func TestCancel(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	started := make(chan struct{})
	q.Handle("long", func(ctx context.Context, _ *Run) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	ctx := context.Background()
	queued, _, _ := q.Enqueue(ctx, "later", nil, nil, "", me)
	if err := q.Cancel(ctx, queued); err != nil {
		t.Fatal(err)
	}
	if j := getJob(t, s, queued); j.State != "cancelled" {
		t.Fatalf("queued job state %s", j.State)
	}
	running, _, _ := q.Enqueue(ctx, "long", nil, nil, "", me)
	_ = q.Start(ctx)
	defer q.Stop()
	<-started
	if err := q.Cancel(ctx, running); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, running, "cancelled")
	if err := q.Cancel(ctx, running); err == nil {
		t.Fatal("cancelling a finished job should fail")
	}
}

func TestIdlePollingWritesNothing(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	time.Sleep(20 * time.Millisecond)
	before := s.WriteCount()
	time.Sleep(200 * time.Millisecond) // about 40 polls on each worker
	if n := s.WriteCount() - before; n != 0 {
		t.Fatalf("idle queue made %d write transactions", n)
	}
}

// 4.15 review: a result saved while the Job runs is kept when it is
// cancelled, and a recovered Job sees it.
func TestSaveResultKeptOnCancel(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	saved := make(chan struct{})
	q.Handle("scan", func(ctx context.Context, r *Run) error {
		if err := r.SaveResult(ctx, map[string]any{"part": 1}); err != nil {
			return err
		}
		close(saved)
		<-ctx.Done()
		return ctx.Err()
	})
	ctx := context.Background()
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	id, _, _ := q.Enqueue(ctx, "scan", nil, nil, "", me)
	<-saved
	if j := getJob(t, s, id); j.State != "running" || j.Result["part"] != float64(1) {
		t.Fatalf("while running: %s %v", j.State, j.Result)
	}
	if err := q.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if j := waitState(t, s, id, "cancelled"); j.Result["part"] != float64(1) {
		t.Fatalf("after cancel: %v", j.Result)
	}
}
