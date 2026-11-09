package jobs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestScheduleStoreHelpers(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	db := s.Read()

	// A kind with no row reads as 0 and off, without an error.
	every, on, err := ScheduleEvery(ctx, db, "no_such_kind")
	if every != 0 || on || err != nil {
		t.Fatalf("unknown kind: %d %v %v", every, on, err)
	}
	if ScheduleEnabled(ctx, db, "no_such_kind") {
		t.Fatal("unknown kind reads as on")
	}

	nextRun := func() (v sql.NullString) {
		if err := db.QueryRowContext(ctx, "SELECT next_run FROM schedules WHERE kind = ?", KindFeedCheck).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	// Give the schedule a known next run so "unchanged" means something.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET next_run = '2030-01-01T00:00:00Z' WHERE kind = ?", KindFeedCheck)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := nextRun()
	if !before.Valid || before.String == "" {
		t.Fatal("next run not set")
	}

	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		return SetScheduleEveryTx(tx, KindFeedCheck, 600, true, database.System, now)
	}); err != nil {
		t.Fatal(err)
	}
	every, on, err = ScheduleEvery(ctx, db, KindFeedCheck)
	if every != 600 || !on || err != nil {
		t.Fatalf("after set: %d %v %v", every, on, err)
	}
	if after := nextRun(); after != before {
		t.Fatalf("next run moved: %v -> %v", before, after)
	}

	if err := s.Write(ctx, func(tx *sql.Tx) error {
		return SetScheduleEnabledTx(tx, KindFeedCheck, false, database.System, now)
	}); err != nil {
		t.Fatal(err)
	}
	if ScheduleEnabled(ctx, db, KindFeedCheck) {
		t.Fatal("still on")
	}
	every, _, _ = ScheduleEvery(ctx, db, KindFeedCheck)
	if every != 600 {
		t.Fatalf("every changed: %d", every)
	}
	if after := nextRun(); after != before {
		t.Fatalf("next run moved by enable toggle: %v", after)
	}
}
