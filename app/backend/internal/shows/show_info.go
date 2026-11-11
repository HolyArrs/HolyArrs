package shows

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Info is what a show's page needs besides the shared Title fields.
type Info struct {
	DetailsSource string          `json:"details_source"`
	EpisodeOrder  string          `json:"episode_order"`
	SeasonChoice  string          `json:"season_choice,omitempty"`
	NewSeasons    string          `json:"new_seasons"`
	Status        string          `json:"status"`
	Network       string          `json:"network,omitempty"`
	AirTime       string          `json:"air_time,omitempty"`
	TimeZone      string          `json:"time_zone,omitempty"`
	Seasons       []SeasonSummary `json:"seasons"`
	Episodes      int             `json:"episodes"`
	Missing       int             `json:"missing"`
	NotOutYet     int             `json:"not_out_yet"`
	InLibrary     int             `json:"in_library"`
	BetterWanted  int             `json:"better_wanted"`
	Downloading   int             `json:"downloading"`
	Failed        int             `json:"failed"`
	NextAirDate   *time.Time      `json:"next_air_date,omitempty"`
	// SeasonCount counts numbered seasons (specials left out); LastAirDate
	// is the most recent episode that has aired.
	SeasonCount int        `json:"season_count"`
	LastAirDate *time.Time `json:"last_air_date,omitempty"`
}

// InfoOf builds a show's Info from its seasons and episodes.
func InfoOf(ctx context.Context, db database.Lister, t mediafiles.Title) (*Info, error) {
	ss, err := Seasons(ctx, db, t.ID, time.Now())
	if err != nil {
		return nil, err
	}
	si := &Info{DetailsSource: t.MetadataSource, EpisodeOrder: strOrEmpty(t.EpisodeOrder), SeasonChoice: strOrEmpty(t.SeasonChoice), NewSeasons: t.NewPartsPolicy,
		Status: t.Status, Network: strOrEmpty(t.Network), AirTime: strOrEmpty(t.AirTime), TimeZone: strOrEmpty(t.TimeZone), Seasons: ss}
	for _, s := range ss {
		if s.Number > 0 {
			si.SeasonCount++
		}
		// Specials count toward "X of Y episodes" only when kept up to date.
		if s.Number > 0 || s.KeepUpToDate {
			si.Episodes += s.Episodes
			si.NotOutYet += s.NotOutYet
			si.InLibrary += s.InLibrary
		}
		si.Missing += s.Missing
		si.BetterWanted += s.BetterWanted
		si.Downloading += s.Downloading
		si.Failed += s.Failed
		if s.NextAirDate != nil && (si.NextAirDate == nil || s.NextAirDate.Before(*si.NextAirDate)) {
			si.NextAirDate = s.NextAirDate
		}
	}
	if si.LastAirDate, err = LastAired(ctx, db, t.ID, time.Now()); err != nil {
		return nil, err
	}
	return si, nil
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// LastAired is the newest air date of a show's episodes on or before now;
// nil when nothing has aired yet.
func LastAired(ctx context.Context, db database.Lister, titleID int64, now time.Time) (*time.Time, error) {
	// Stored times share one fixed-width UTC layout, so SQLite can pick the
	// newest aired one instead of the server parsing every episode.
	rows, err := db.QueryContext(ctx, `SELECT max(air_date) FROM items WHERE title_id = ? AND kind = 'episode' AND air_date IS NOT NULL AND air_date <= ?`, titleID, database.FormatTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var last *time.Time
	for rows.Next() {
		var air sql.NullString
		if err := rows.Scan(&air); err != nil {
			return nil, err
		}
		at, err := time.Parse(database.TimeLayout, air.String)
		if err != nil || at.After(now) {
			continue
		}
		if last == nil || at.After(*last) {
			a := at
			last = &a
		}
	}
	return last, rows.Err()
}
