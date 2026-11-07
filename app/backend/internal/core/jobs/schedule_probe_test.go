package jobs

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func dueSchedule(t *testing.T, s *database.Store, kind string, every int64, now time.Time) Schedule {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET enabled = (kind = ?), every_seconds = ?, next_run = ?, updated_at = ?", kind, every, database.FormatTime(now), database.FormatTime(now.Add(-time.Hour)))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	scs, err := database.Select[Schedule](context.Background(), s.Read(), "schedules", "kind = ?", kind)
	if err != nil || len(scs) != 1 {
		t.Fatal(err)
	}
	return scs[0]
}

func jobCount(t *testing.T, s *database.Store, kind string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestScheduleProbeNoWorkUsesMemoryOnlyUntilNextProbe(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	var calls atomic.Int32
	var changes atomic.Int32
	q.opt.OnChange = func(Job) { changes.Add(1) }
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
		calls.Add(1)
		return false, nil, nil
	})

	writes := s.WriteCount()
	_, eventsBefore, err := s.Events(context.Background(), database.EventFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if kinds, err := q.RunSchedules(context.Background(), now); err != nil || len(kinds) != 0 {
		t.Fatalf("first probe: kinds %v err %v", kinds, err)
	}
	if calls.Load() != 1 || s.WriteCount() != writes || jobCount(t, s, sc.Kind) != 0 || changes.Load() != 0 {
		t.Fatalf("false probe calls=%d writes=%d jobs=%d changes=%d", calls.Load(), s.WriteCount()-writes, jobCount(t, s, sc.Kind), changes.Load())
	}
	_, eventsAfter, err := s.Events(context.Background(), database.EventFilter{Limit: 1})
	if err != nil || eventsAfter != eventsBefore {
		t.Fatalf("false probe events: before=%d after=%d err=%v", eventsBefore, eventsAfter, err)
	}
	wantNext := now.Add(15 * time.Minute)
	if got := q.EffectiveScheduleNext(sc); got == nil || !got.Equal(wantNext) {
		t.Fatalf("effective next %v, want %v", got, wantNext)
	}
	if _, err := q.RunSchedules(context.Background(), now.Add(14*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || s.WriteCount() != writes {
		t.Fatalf("reprobed early: calls=%d writes=%d", calls.Load(), s.WriteCount()-writes)
	}
	if _, err := q.RunSchedules(context.Background(), wantNext); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || s.WriteCount() != writes {
		t.Fatalf("interval reprobe: calls=%d writes=%d", calls.Load(), s.WriteCount()-writes)
	}
}

func TestScheduleProbeWatermarkInvalidatesOnEditAndRestart(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	var calls atomic.Int32
	probe := func(context.Context, time.Time) (bool, any, error) { calls.Add(1); return false, nil, nil }
	q := New(s, Options{})
	q.ProbeSchedule(sc.Kind, probe)
	_, _ = q.RunSchedules(context.Background(), now)

	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET every_seconds = ?, updated_at = ? WHERE kind = ?", 1800, database.FormatTime(now.Add(time.Second)), sc.Kind)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RunSchedules(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("edited schedule did not invalidate watermark: %d calls", calls.Load())
	}

	q2 := New(s, Options{})
	q2.ProbeSchedule(sc.Kind, probe)
	if _, err := q2.RunSchedules(context.Background(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("new queue did not reprobe: %d calls", calls.Load())
	}
}

type scheduleOffGate struct{ kind string }

func (g scheduleOffGate) Active() bool { return true }
func (g scheduleOffGate) Held(context.Context, database.Querier, string, *database.Subject) bool {
	return false
}
func (g scheduleOffGate) KindOff(kind string) bool { return kind == g.kind }

type scheduleToggleGate struct {
	kind string
	off  atomic.Bool
}

func (g *scheduleToggleGate) Active() bool { return true }
func (g *scheduleToggleGate) Held(_ context.Context, _ database.Querier, kind string, _ *database.Subject) bool {
	return kind == g.kind && g.off.Load()
}
func (g *scheduleToggleGate) KindOff(kind string) bool { return kind == g.kind && g.off.Load() }

func TestScheduleProbePartOffRunsBeforeProbe(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	q.SetGate(scheduleOffGate{kind: sc.Kind})
	var calls atomic.Int32
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { calls.Add(1); return false, nil, nil })
	writes := s.WriteCount()
	if kinds, err := q.RunSchedules(context.Background(), now); err != nil || len(kinds) != 0 {
		t.Fatalf("part off: kinds %v err %v", kinds, err)
	}
	if calls.Load() != 0 || s.WriteCount() != writes || jobCount(t, s, sc.Kind) != 0 {
		t.Fatalf("part-off schedule probed or wrote: calls=%d writes=%d jobs=%d", calls.Load(), s.WriteCount()-writes, jobCount(t, s, sc.Kind))
	}
	if got := q.EffectiveScheduleNext(sc); got == nil || !got.Equal(*sc.NextRun) {
		t.Fatalf("part-off next moved: %v, stored %v", got, sc.NextRun)
	}
}

func TestScheduleProbeErrorAndRunNowUseDurableJobs(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	var calls atomic.Int32
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
		calls.Add(1)
		return false, "untrusted", errors.New("uncertain")
	})
	if kinds, err := q.RunSchedules(context.Background(), now); err != nil || len(kinds) != 1 || kinds[0] != sc.Kind {
		t.Fatalf("probe error: kinds %v err %v", kinds, err)
	}
	if calls.Load() != 1 || jobCount(t, s, sc.Kind) != 1 {
		t.Fatalf("probe error was not durable: calls=%d jobs=%d", calls.Load(), jobCount(t, s, sc.Kind))
	}
	if len(q.prepared) != 0 {
		t.Fatalf("probe error retained an untrusted prepared value: %v", q.prepared)
	}

	other := dueSchedule(t, s, "health_check", 60, now)
	q.ProbeSchedule(other.Kind, func(context.Context, time.Time) (bool, any, error) {
		t.Fatal("Run now called the schedule probe")
		return false, nil, nil
	})
	if _, created, err := q.Enqueue(context.Background(), other.Kind, nil, map[string]any{"by_hand": true}, "run-now:"+other.Kind, database.System); err != nil || !created {
		t.Fatalf("run now: created=%v err=%v", created, err)
	}
	if jobCount(t, s, other.Kind) != 1 {
		t.Fatal("Run now did not create a durable Job")
	}
}

func TestPanickingScheduleProbeQueuesDurableJob(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	type probeSecret string
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
		panic(probeSecret("do-not-store-this"))
	})
	kinds, err := q.RunSchedules(context.Background(), now)
	if err != nil || len(kinds) != 1 || kinds[0] != sc.Kind {
		t.Fatalf("panicking probe: kinds=%v err=%v", kinds, err)
	}
	if jobCount(t, s, sc.Kind) != 1 || len(q.prepared) != 0 {
		t.Fatalf("panicking probe did not fail open: jobs=%d prepared=%v", jobCount(t, s, sc.Kind), q.prepared)
	}
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM jobs WHERE kind = ?", sc.Kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if job := getJob(t, s, id); job.Error != nil {
		t.Fatalf("probe panic payload was stored on the Job: %+v", job.Error)
	}
}

func TestSlowScheduleProbeDoesNotStarveLaterSchedule(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	dueSchedule(t, s, "refresh_titles", 86400, now)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET enabled = 1, next_run = ? WHERE kind = 'health_check'", database.FormatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	q := New(s, Options{})
	release := make(chan struct{})
	q.ProbeSchedule("refresh_titles", func(context.Context, time.Time) (bool, any, error) {
		<-release
		return false, nil, nil
	})
	q.ProbeSchedule("health_check", func(context.Context, time.Time) (bool, any, error) { return true, nil, nil })
	done := make(chan error, 1)
	go func() {
		_, err := q.RunSchedules(context.Background(), now)
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for jobCount(t, s, "health_check") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if jobCount(t, s, "health_check") != 1 {
		close(release)
		<-done
		t.Fatal("later schedule was starved by a slow probe")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCancelledSchedulePassDoesNotWaitForStuckProbeOrStartAnother(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	dueSchedule(t, s, "refresh_titles", 86400, now)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET enabled = 1, next_run = ? WHERE kind = 'health_check'", database.FormatTime(now))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	q := New(s, Options{})
	release := make(chan struct{})
	var stuckCalls atomic.Int32
	q.ProbeSchedule("refresh_titles", func(context.Context, time.Time) (bool, any, error) {
		stuckCalls.Add(1)
		<-release // deliberately ignores cancellation
		return false, nil, nil
	})
	q.ProbeSchedule("health_check", func(context.Context, time.Time) (bool, any, error) { return true, nil, nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := q.RunSchedules(ctx, now)
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for jobCount(t, s, "health_check") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if jobCount(t, s, "health_check") != 1 {
		close(release)
		t.Fatal("later schedule was not queued before cancellation")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunSchedules error %v, want context canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		close(release)
		t.Fatal("RunSchedules waited for a probe that ignored cancellation")
	}
	if _, err := q.RunSchedules(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if stuckCalls.Load() != 1 {
		t.Fatalf("stuck probe started %d times", stuckCalls.Load())
	}
	close(release)
	deadline = time.Now().Add(time.Second)
	for {
		q.mu.Lock()
		inFlight := len(q.scheduleProbeInFlight)
		q.mu.Unlock()
		if inFlight == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("finished probe remains in flight: %d", inFlight)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScheduleProbeDiscardsResultAfterScheduleChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *database.Store, Schedule, time.Time)
		check  func(*testing.T, *database.Store, Schedule, time.Time)
	}{
		{
			name: "disabled",
			change: func(t *testing.T, s *database.Store, sc Schedule, now time.Time) {
				t.Helper()
				if err := s.Write(context.Background(), func(tx *sql.Tx) error {
					_, err := tx.Exec("UPDATE schedules SET enabled = 0, updated_at = ? WHERE id = ?", database.FormatTime(now.Add(time.Second)), sc.ID)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, s *database.Store, sc Schedule, _ time.Time) {
				t.Helper()
				var enabled bool
				if err := s.Read().QueryRow("SELECT enabled FROM schedules WHERE id = ?", sc.ID).Scan(&enabled); err != nil || enabled {
					t.Fatalf("disabled schedule changed: enabled=%v err=%v", enabled, err)
				}
			},
		},
		{
			name: "interval edited",
			change: func(t *testing.T, s *database.Store, sc Schedule, now time.Time) {
				t.Helper()
				if err := s.Write(context.Background(), func(tx *sql.Tx) error {
					_, err := tx.Exec("UPDATE schedules SET every_seconds = 1800, next_run = ?, updated_at = ? WHERE id = ?",
						database.FormatTime(now.Add(30*time.Minute)), database.FormatTime(now.Add(time.Second)), sc.ID)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, s *database.Store, sc Schedule, now time.Time) {
				t.Helper()
				var every int64
				var next string
				if err := s.Read().QueryRow("SELECT every_seconds, next_run FROM schedules WHERE id = ?", sc.ID).Scan(&every, &next); err != nil {
					t.Fatal(err)
				}
				if every != 1800 || next != database.FormatTime(now.Add(30*time.Minute)) {
					t.Fatalf("edited clock overwritten: every=%d next=%s", every, next)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t, t.TempDir())
			t.Cleanup(func() { _ = s.Close() })
			now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
			sc := dueSchedule(t, s, "feed_check", 900, now)
			q := New(s, Options{})
			started := make(chan struct{})
			release := make(chan struct{})
			q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
				close(started)
				<-release
				return true, "stale", nil
			})
			done := make(chan error, 1)
			go func() {
				_, err := q.RunSchedules(context.Background(), now)
				done <- err
			}()
			<-started
			tc.change(t, s, sc, now)
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if jobCount(t, s, sc.Kind) != 0 || len(q.prepared) != 0 {
				t.Fatalf("stale probe created work: jobs=%d prepared=%v", jobCount(t, s, sc.Kind), q.prepared)
			}
			tc.check(t, s, sc, now)
		})
	}
}

func TestScheduleProbeDiscardsResultWhenPartTurnsOff(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	gate := &scheduleToggleGate{kind: sc.Kind}
	q.SetGate(gate)
	started := make(chan struct{})
	release := make(chan struct{})
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
		close(started)
		<-release
		return true, "stale", nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := q.RunSchedules(context.Background(), now)
		done <- err
	}()
	<-started
	gate.off.Store(true)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if jobCount(t, s, sc.Kind) != 0 || len(q.prepared) != 0 {
		t.Fatalf("part-off probe created work: jobs=%d prepared=%v", jobCount(t, s, sc.Kind), q.prepared)
	}
	fresh := dueScheduleRecord(t, s, sc.Kind)
	if fresh.NextRun == nil || !fresh.NextRun.Equal(*sc.NextRun) {
		t.Fatalf("part-off schedule clock moved: %v, want %v", fresh.NextRun, sc.NextRun)
	}
}

func dueScheduleRecord(t *testing.T, s *database.Store, kind string) Schedule {
	t.Helper()
	scs, err := database.Select[Schedule](context.Background(), s.Read(), "schedules", "kind = ?", kind)
	if err != nil || len(scs) != 1 {
		t.Fatalf("schedule %s: %v %v", kind, scs, err)
	}
	return scs[0]
}

func TestScheduleProbePreparedValueIsOneShotAndRetriesRefetch(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Now().UTC()
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, fastOpts())
	prepared := &struct{ value string }{"already fetched"}
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { return true, prepared, nil })
	var attempts atomic.Int32
	q.Handle(sc.Kind, func(_ context.Context, run *Run) error {
		n := attempts.Add(1)
		got := run.TakePrepared()
		if n == 1 {
			second := run.TakePrepared()
			if got != prepared || second != nil {
				t.Errorf("first attempt prepared values: first=%v second=%v", got, second)
			}
			return errors.New("retry")
		}
		if got != nil {
			t.Errorf("retry reused prepared value: %v", got)
		}
		return nil
	})
	kinds, err := q.RunSchedules(context.Background(), now)
	if err != nil || len(kinds) != 1 {
		t.Fatalf("schedule: %v %v", kinds, err)
	}
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM jobs WHERE kind = ?", sc.Kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, id, "done")
	if attempts.Load() != 2 {
		t.Fatalf("attempts %d", attempts.Load())
	}
}

func TestScheduleProbePreparedValueClearedOnCancel(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { return true, "prepared", nil })
	if _, err := q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM jobs WHERE kind = ?", sc.Kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, ok := q.prepared[id]; !ok {
		t.Fatal("new scheduled Job has no prepared value")
	}
	if err := q.Cancel(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(q.prepared) != 0 {
		t.Fatalf("prepared values remain after cancel: %v", q.prepared)
	}
}

func TestScheduleProbePreparedValueIsNotRecoveredAfterRestart(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q1 := New(s, Options{})
	q1.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { return true, "old process", nil })
	if _, err := q1.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}

	q2 := New(s, fastOpts())
	seen := make(chan any, 1)
	q2.Handle(sc.Kind, func(_ context.Context, run *Run) error {
		seen <- run.TakePrepared()
		return nil
	})
	if err := q2.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q2.Stop)
	select {
	case got := <-seen:
		if got != nil {
			t.Fatalf("restart recovered process-local prepared value: %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restarted Queue did not run the durable Job")
	}
}

func TestExpiredSchedulePreparedValueIsDiscarded(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Now().UTC()
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, fastOpts())
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { return true, "stale", nil })
	if _, err := q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM jobs WHERE kind = ?", sc.Kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	entry := q.prepared[id]
	entry.expires = time.Now().Add(-time.Second)
	q.prepared[id] = entry
	seen := make(chan any, 1)
	q.Handle(sc.Kind, func(_ context.Context, run *Run) error {
		seen <- run.TakePrepared()
		return nil
	})
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	select {
	case got := <-seen:
		if got != nil {
			t.Fatalf("expired prepared value reached handler: %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scheduled Job did not run")
	}
}

func TestQueueStopClearsProcessLocalScheduleState(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	var calls atomic.Int32
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
		if calls.Add(1) == 1 {
			return false, nil, nil
		}
		return true, "prepared", nil
	})
	if _, err := q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	q.Stop()
	if len(q.scheduleNoop) != 0 || len(q.prepared) != 0 {
		t.Fatalf("Stop kept process state: no-op=%v prepared=%v", q.scheduleNoop, q.prepared)
	}
	if _, err := q.RunSchedules(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(q.prepared) != 1 {
		t.Fatalf("same Queue did not reprobe after Stop: calls=%d prepared=%v", calls.Load(), q.prepared)
	}
	q.Stop()
	if len(q.prepared) != 0 {
		t.Fatalf("Stop kept queued prepared value: %v", q.prepared)
	}
}

func TestScheduleProbePreparedValueDiscardedOnDedupeAndCancel(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	id, created, err := q.Enqueue(context.Background(), sc.Kind, nil, nil, "schedule:"+sc.Kind, database.System)
	if err != nil || !created {
		t.Fatalf("seed job: created=%v err=%v", created, err)
	}
	var probes atomic.Int32
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
		return true, probes.Add(1), nil
	})
	if _, err := q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if probes.Load() != 0 {
		t.Fatalf("live scheduled Job did not suppress probe: %d calls", probes.Load())
	}
	if _, ok := q.prepared[id]; ok {
		t.Fatal("prepared value attached to an existing deduped Job")
	}
	if err := q.Cancel(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if fresh := dueScheduleRecord(t, s, sc.Kind); fresh.NextRun == nil || !fresh.NextRun.Equal(*sc.NextRun) {
		t.Fatalf("dedupe moved the schedule clock: %v, want %v", fresh.NextRun, sc.NextRun)
	}
	if _, err := q.RunSchedules(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var nextID int64
	if err := s.Read().QueryRow("SELECT id FROM jobs WHERE kind = ? AND state = 'queued'", sc.Kind).Scan(&nextID); err != nil {
		t.Fatal(err)
	}
	if nextID == id || q.prepared[nextID].value != int32(1) {
		t.Fatalf("follow-up Job id=%d prepared=%v, old id=%d", nextID, q.prepared[nextID], id)
	}
	if len(q.prepared) != 1 {
		t.Fatalf("unexpected prepared values: %v", q.prepared)
	}
	q.clearPrepared(nextID)
}

func TestQueuedOrRunningScheduledJobSkipsProbeAndWrites(t *testing.T) {
	for _, state := range []string{"queued", "running"} {
		t.Run(state, func(t *testing.T) {
			s := newStore(t, t.TempDir())
			t.Cleanup(func() { _ = s.Close() })
			now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
			sc := dueSchedule(t, s, "feed_check", 900, now)
			q := New(s, Options{})
			id, created, err := q.Enqueue(context.Background(), sc.Kind, nil, nil, "schedule:"+sc.Kind, database.System)
			if err != nil || !created {
				t.Fatalf("seed job: created=%v err=%v", created, err)
			}
			if state == "running" {
				if err := s.Write(context.Background(), func(tx *sql.Tx) error {
					_, err := tx.Exec("UPDATE jobs SET state = 'running' WHERE id = ?", id)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
				calls.Add(1)
				return false, nil, nil
			})
			writes := s.WriteCount()
			for _, at := range []time.Time{now, now.Add(time.Minute)} {
				if _, err := q.RunSchedules(context.Background(), at); err != nil {
					t.Fatal(err)
				}
			}
			if calls.Load() != 0 || s.WriteCount() != writes {
				t.Fatalf("live %s Job: probes=%d writes=%d", state, calls.Load(), s.WriteCount()-writes)
			}
			if fresh := dueScheduleRecord(t, s, sc.Kind); fresh.NextRun == nil || !fresh.NextRun.Equal(*sc.NextRun) {
				t.Fatalf("live %s Job moved clock: %v, want %v", state, fresh.NextRun, sc.NextRun)
			}
			if err := s.Write(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE jobs SET state = 'done' WHERE id = ?", id)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := q.RunSchedules(context.Background(), now.Add(2*time.Minute)); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatalf("finished %s Job did not release probe: %d calls", state, calls.Load())
			}
		})
	}
}

func TestHeldScheduledJobDropsPreparedValue(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "feed_check", 900, now)
	q := New(s, Options{})
	gate := &scheduleToggleGate{kind: sc.Kind}
	q.SetGate(gate)
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { return true, "prepared", nil })
	if _, err := q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM jobs WHERE kind = ?", sc.Kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	gate.off.Store(true)
	if _, claimed, err := q.claim(context.Background(), ""); err != nil || claimed {
		t.Fatalf("held claim: claimed=%v err=%v", claimed, err)
	}
	if _, ok := q.prepared[id]; ok {
		t.Fatal("held Job retained stale prepared data")
	}
}
