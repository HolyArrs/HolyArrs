package shows

import (
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A show kept only in part needs to know how many episodes are kept.
func TestTallyCountsKeptEpisodes(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	at := func(t time.Time) sql.NullString {
		return sql.NullString{String: t.UTC().Format(database.TimeLayout), Valid: true}
	}
	var ss SeasonSummary
	ss.tally(at(now.Add(-time.Hour)), "", true, "in_library", true, now)
	ss.tally(at(now.Add(-time.Hour)), "", false, "wanted", false, now)
	ss.tally(at(now.Add(48*time.Hour)), "", true, "wanted", false, now)
	if ss.Kept != 2 {
		t.Fatalf("kept = %d, want 2", ss.Kept)
	}
}
