package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"
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
