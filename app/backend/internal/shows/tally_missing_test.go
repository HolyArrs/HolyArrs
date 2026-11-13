package shows

import (
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A wanted episode counts as missing only once its air time has
// passed; one airing later (tomorrow, say) is not out yet.
func TestTallyMissingOnlyWhenAired(t *testing.T) {
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	at := func(t time.Time) sql.NullString {
		return sql.NullString{String: t.UTC().Format(database.TimeLayout), Valid: true}
	}
	var ss SeasonSummary
	ss.tally(at(now.Add(-time.Hour)), "", true, "wanted", now)   // aired: missing
	ss.tally(at(now.Add(26*time.Hour)), "", true, "wanted", now) // tomorrow: not out yet
	ss.tally(sql.NullString{}, "", true, "wanted", now)          // no air date: not missing
	ss.tally(at(now.Add(-time.Hour)), "", false, "wanted", now)  // not kept: not missing
	if ss.Missing != 1 || ss.NotOutYet != 2 || ss.Aired != 2 {
		t.Fatalf("missing=%d not_out_yet=%d aired=%d, want 1/2/2", ss.Missing, ss.NotOutYet, ss.Aired)
	}
}

// The next episode's own air day travels with the next air date, so a
// season header can show the same day as the episode row.
func TestTallyNextAirDayIsOfEarliestUnaired(t *testing.T) {
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	at := func(t time.Time) sql.NullString {
		return sql.NullString{String: t.UTC().Format(database.TimeLayout), Valid: true}
	}
	var ss SeasonSummary
	ss.tally(at(now.Add(-time.Hour)), "2026-11-12", true, "wanted", now)
	ss.tally(at(now.Add(200*time.Hour)), "2026-11-22", true, "wanted", now)
	ss.tally(at(time.Date(2026, 11, 16, 1, 30, 0, 0, time.UTC)), "2026-11-15", true, "wanted", now)
	if ss.NextAirDay != "2026-11-15" || ss.NextAirDate == nil || ss.NextAirDate.Day() != 16 {
		t.Fatalf("next_air_day=%q next_air_date=%v, want 2026-11-15 with the 16th", ss.NextAirDay, ss.NextAirDate)
	}
}
