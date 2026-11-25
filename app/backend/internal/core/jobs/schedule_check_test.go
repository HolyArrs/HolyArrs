package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func readSchedule(t *testing.T, s *database.Store, kind string) Schedule {
	t.Helper()
	scs, err := database.Select[Schedule](context.Background(), s.Read(), "schedules", "kind = ?", kind)
	if err != nil || len(scs) != 1 {
		t.Fatal(err)
	}
	return scs[0]
}

// A no-work check keeps when and why on the schedule, never moves last_run,
// writes again only when the reason changes or the kept one is an hour old,
// and LastScheduleCheck always answers the newest check.
func TestScheduleCheckKeepsReason(t *testing.T) {
	m := time.Minute
	cases := []struct {
		name    string
		reasons []any           // what each probe returns as prepared
		at      []time.Duration // when each pass runs, after now
		writes  int64           // writes in all
		skip    string          // kept reason at the end
		checked time.Duration   // kept check at the end
		newest  time.Duration   // LastScheduleCheck at the end
	}{
		{"no reason keeps nothing", []any{nil}, []time.Duration{0}, 1, "nothing", 0, 0},
		{"odd prepared keeps nothing", []any{"not a reason"}, []time.Duration{0}, 1, "nothing", 0, 0},
		{"reason kept", []any{SkipReason("setting_off")}, []time.Duration{0}, 1, "setting_off", 0, 0},
		{"same reason inside the hour stays in memory", []any{SkipReason("nobody"), SkipReason("nobody")},
			[]time.Duration{0, 5 * m}, 1, "nobody", 0, 5 * m},
		{"changed reason writes", []any{SkipReason("setting_off"), SkipReason("nobody")},
			[]time.Duration{0, 5 * m}, 2, "nobody", 5 * m, 5 * m},
		{"hour old check writes again", []any{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
			[]time.Duration{0, 5 * m, 10 * m, 15 * m, 20 * m, 25 * m, 30 * m, 35 * m, 40 * m, 45 * m, 50 * m, 55 * m, 60 * m}, 2, "nothing", 60 * m, 60 * m},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newStore(t, t.TempDir())
			t.Cleanup(func() { _ = s.Close() })
			now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
			sc := dueSchedule(t, s, "sync_plex_watchlists", 300, now)
			q := New(s, Options{})
			i := 0
			q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) {
				r := c.reasons[i]
				i++
				return false, r, nil
			})
			base := s.WriteCount()
			for _, d := range c.at {
				if _, err := q.RunSchedules(context.Background(), now.Add(d)); err != nil {
					t.Fatal(err)
				}
			}
			if got := s.WriteCount() - base; got != c.writes {
				t.Fatalf("writes %d, want %d", got, c.writes)
			}
			if i != len(c.reasons) {
				t.Fatalf("probed %d times, want %d", i, len(c.reasons))
			}
			got := readSchedule(t, s, sc.Kind)
			if got.LastRun != nil || jobCount(t, s, sc.Kind) != 0 {
				t.Fatalf("no-work check moved last_run %v or queued a Job", got.LastRun)
			}
			if got.LastSkip == nil || *got.LastSkip != c.skip || got.LastChecked == nil || !got.LastChecked.Equal(now.Add(c.checked)) {
				t.Fatalf("kept %v at %v, want %s at %v", got.LastSkip, got.LastChecked, c.skip, now.Add(c.checked))
			}
			at, _ := q.LastScheduleCheck(got)
			if at == nil || !at.Equal(now.Add(c.newest)) {
				t.Fatalf("newest check %v, want %v", at, now.Add(c.newest))
			}
			if next := q.EffectiveScheduleNext(got); next == nil || !next.Equal(now.Add(c.newest+5*m)) {
				t.Fatalf("next %v, want %v", next, now.Add(c.newest+5*m))
			}
		})
	}
}

// After a restart the kept check still says when the schedule is next due
// and why, and a real run afterwards (updated_at moves) drops it.
func TestScheduleCheckSurvivesRestartUntilRun(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	sc := dueSchedule(t, s, "sync_plex_watchlists", 300, now)
	q := New(s, Options{})
	q.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { return false, SkipReason("setting_off"), nil })
	if _, err := q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	q2 := New(s, Options{})
	got := readSchedule(t, s, sc.Kind)
	if next := q2.EffectiveScheduleNext(got); next == nil || !next.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("after restart next %v", next)
	}
	if at, why := q2.LastScheduleCheck(got); at == nil || why != "setting_off" {
		t.Fatalf("after restart check %v %q", at, why)
	}
	probes := 0
	q2.ProbeSchedule(sc.Kind, func(context.Context, time.Time) (bool, any, error) { probes++; return true, nil, nil })
	if kinds, err := q2.RunSchedules(context.Background(), now.Add(4*time.Minute)); err != nil || len(kinds) != 0 || probes != 0 {
		t.Fatalf("probed inside the kept check: kinds %v probes %d err %v", kinds, probes, err)
	}
	if kinds, err := q2.RunSchedules(context.Background(), now.Add(5*time.Minute)); err != nil || len(kinds) != 1 {
		t.Fatalf("run: %v %v", kinds, err)
	}
	got = readSchedule(t, s, sc.Kind)
	if _, ok := checkedNext(got); ok {
		t.Fatal("a real run kept the old check in force")
	}
	if got.LastRun == nil || !got.LastRun.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("last_run %v", got.LastRun)
	}
}
