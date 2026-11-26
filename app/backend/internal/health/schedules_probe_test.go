package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

func TestSchedulesShowEffectiveProbeTimeWithoutChangingHistory(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	now := time.Now().UTC().Add(time.Second)
	f.q.ProbeSchedule("feed_check", func(context.Context, time.Time) (bool, any, error) { return false, nil, nil })
	if _, err := f.q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var storedNext, lastRun sql.NullString
	if err := f.s.Read().QueryRow("SELECT next_run, last_run FROM schedules WHERE kind = 'feed_check'").Scan(&storedNext, &lastRun); err != nil {
		t.Fatal(err)
	}
	var answer schedulesAnswer
	if err := json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &answer); err != nil {
		t.Fatal(err)
	}
	v := scheduleNamed(t, answer, "feed_check")
	if v.NextRun == nil || v.NextRun.Before(now.Add(14*time.Minute)) {
		t.Fatalf("effective next %v", v.NextRun)
	}
	if v.LastRun != nil || v.LastJob != "" || lastRun.Valid {
		t.Fatalf("no-op probe falsified history: view=%+v stored last=%v", v, lastRun)
	}
	if storedNext.Valid {
		t.Fatalf("stored next changed: %v", storedNext)
	}
}

func TestRunScheduleBypassesProbeWithoutMovingClock(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	var calls int
	f.q.ProbeSchedule("feed_check", func(context.Context, time.Time) (bool, any, error) {
		calls++
		return false, nil, nil
	})
	var nextBefore, lastBefore sql.NullString
	if err := f.s.Read().QueryRow("SELECT next_run, last_run FROM schedules WHERE kind = 'feed_check'").Scan(&nextBefore, &lastBefore); err != nil {
		t.Fatal(err)
	}
	do(t, "POST", f.srv.URL+"/api/v1/schedules/feed_check/run", "", 202)
	var nextAfter, lastAfter sql.NullString
	if err := f.s.Read().QueryRow("SELECT next_run, last_run FROM schedules WHERE kind = 'feed_check'").Scan(&nextAfter, &lastAfter); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("Run now called the schedule probe %d times", calls)
	}
	if nextAfter != nextBefore || lastAfter != lastBefore {
		t.Fatalf("Run now moved clock: next %v -> %v, last %v -> %v", nextBefore, nextAfter, lastBefore, lastAfter)
	}
}

// The page gets the newest no-work check and why, and for the newest Job
// why it did nothing and how long it took.
func TestSchedulesShowCheckReasonNoteAndDuration(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	now := time.Now().UTC().Add(time.Second)
	f.q.ProbeSchedule("feed_check", func(context.Context, time.Time) (bool, any, error) {
		return false, jobs.SkipReason("setting_off"), nil
	})
	if _, err := f.q.RunSchedules(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, params, priority, state, attempt, max_attempts, run_after, started_at, ended_at, result, asked_by)
			VALUES ('2026-11-19T10:00:00Z', '{"kind":"system"}', '2026-11-19T10:00:00Z', '{"kind":"system"}', 'feed_check', '{}', 'normal', 'done', 1, 5, '2026-11-19T10:00:00Z',
			'2026-11-19T10:00:00Z', '2026-11-19T10:00:02.5Z', '{"skipped":"setting_off"}', '{"kind":"user"}')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var answer schedulesAnswer
	if err := json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &answer); err != nil {
		t.Fatal(err)
	}
	v := scheduleNamed(t, answer, "feed_check")
	if v.LastChecked == nil || !v.LastChecked.Equal(now) || v.LastSkip != "setting_off" || v.LastRun != nil {
		t.Fatalf("check %v %q last_run %v", v.LastChecked, v.LastSkip, v.LastRun)
	}
	if v.LastJob != "done" || v.LastJobNote != "setting_off" || v.LastJobSeconds == nil || *v.LastJobSeconds != 2.5 {
		t.Fatalf("job %q note %q secs %v", v.LastJob, v.LastJobNote, v.LastJobSeconds)
	}
}
