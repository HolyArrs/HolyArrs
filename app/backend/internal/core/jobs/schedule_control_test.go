package jobs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestUpdateScheduleTurnsOffAndCancelsOnlyQueuedScheduledWork(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	q := New(s, Options{})
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	scheduled, _, err := q.EnqueueAt(ctx, "refresh_titles", nil, map[string]any{"scheduled": true}, "schedule:refresh_titles", database.System, future)
	if err != nil {
		t.Fatal(err)
	}
	manual, _, err := q.EnqueueAt(ctx, "refresh_titles", nil, map[string]any{"by_hand": true}, "refresh_titles:by_hand", database.System, future)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := q.EnqueueAt(ctx, "refresh_shows", nil, map[string]any{"scheduled": true}, "schedule:refresh_shows", database.System, future)
	if err != nil {
		t.Fatal(err)
	}

	off := false
	if err := q.UpdateSchedule(ctx, "refresh_titles", nil, &off, database.System, time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if got := dueScheduleRecord(t, s, "refresh_titles"); got.Enabled {
		t.Fatal("refresh_titles stayed enabled")
	}
	if got := getJob(t, s, scheduled); got.State != "cancelled" {
		t.Fatalf("scheduled job state %q", got.State)
	}
	if got := getJob(t, s, manual); got.State != "queued" {
		t.Fatalf("manual job state %q", got.State)
	}
	if got := getJob(t, s, other); got.State != "queued" {
		t.Fatalf("other job state %q", got.State)
	}
}

func TestUpdateSchedulePersistsOffAndReenablesExactlyOneFuturePass(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	q := New(s, Options{})
	ctx := context.Background()
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	off := false
	if err := q.UpdateSchedule(ctx, "refresh_titles", nil, &off, database.System, at); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newStore(t, dir)
	t.Cleanup(func() { _ = reopened.Close() })
	if got := dueScheduleRecord(t, reopened, "refresh_titles"); got.Enabled {
		t.Fatal("off did not survive reopen")
	}

	q = New(reopened, Options{})
	on := true
	if err := q.UpdateSchedule(ctx, "refresh_titles", nil, &on, database.System, at); err != nil {
		t.Fatal(err)
	}
	got := dueScheduleRecord(t, reopened, "refresh_titles")
	want := at.Add(time.Duration(got.EverySeconds) * time.Second)
	if !got.Enabled || got.NextRun == nil || !got.NextRun.Equal(want) {
		t.Fatalf("re-enabled schedule: enabled=%v next=%v want=%v", got.Enabled, got.NextRun, want)
	}
	var jobs int
	if err := reopened.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = 'refresh_titles'").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("re-enable queued %d catch-up jobs: %v", jobs, err)
	}
}

func TestUpdateScheduleIntervalOnlyClientStillWorks(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	q := New(s, Options{})
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	every := int64(2 * time.Hour / time.Second)
	if err := q.UpdateSchedule(context.Background(), "refresh_titles", &every, nil, database.System, at); err != nil {
		t.Fatal(err)
	}
	got := dueScheduleRecord(t, s, "refresh_titles")
	if !got.Enabled || got.EverySeconds != every || got.NextRun == nil || !got.NextRun.Equal(at.Add(2*time.Hour)) {
		t.Fatalf("interval-only update: %+v", got)
	}
}

func TestUpdateScheduleMissingKind(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	q := New(s, Options{})
	off := false
	if err := q.UpdateSchedule(context.Background(), "missing", nil, &off, database.System, time.Now()); err != sql.ErrNoRows {
		t.Fatalf("error %v, want sql.ErrNoRows", err)
	}
}
