package jobs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestRunSchedules(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	q := New(s, Options{})
	ctx := context.Background()
	// This test follows the two day-long seeded schedules; the 15-minute
	// Outside list check, the 12-hour show and artist refreshes, the search schedules and the subtitle schedules are left out so "not due yet" stays true.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET enabled = 0 WHERE kind IN ('sync_outside_lists', 'feed_check', 'grab_held', 'search_missing', 'rescan_library', 'clean_holding', 'health_check', 'refresh_shows', 'refresh_artists', 'artist_pictures', 'subtitle_missing', 'subtitle_better', 'clean_history', 'sync_plex_watchlists', 'check_bridges')")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	kinds, err := q.RunSchedules(ctx, now)
	if err != nil || len(kinds) != 2 {
		t.Fatalf("first run %v %v", kinds, err)
	}
	writes := s.WriteCount()
	if kinds, _ := q.RunSchedules(ctx, now.Add(time.Hour)); len(kinds) != 0 {
		t.Fatalf("not due yet: %v", kinds)
	}
	if s.WriteCount() != writes {
		t.Fatal("a run with nothing due wrote to the database")
	}
	// Let the first Jobs end, then the next day's run carries the last start.
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("UPDATE jobs SET state = 'done'"); return err }); err != nil {
		t.Fatal(err)
	}
	kinds, _ = q.RunSchedules(ctx, now.Add(25*time.Hour))
	if len(kinds) != 2 {
		t.Fatalf("next day %v", kinds)
	}
	js, _ := database.Select[Job](ctx, s.Read(), "jobs", "kind = 'refresh_titles' AND state = 'queued'")
	if len(js) != 1 || js[0].Params["last_start"] != now.Format(time.RFC3339Nano) || js[0].Params["scheduled"] != true {
		t.Fatalf("jobs %+v", js)
	}
}
