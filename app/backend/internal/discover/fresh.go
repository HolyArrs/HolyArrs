package discover

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

type freshRowKey struct{}

// WithFreshRow names the row type the cards are joined for, so their strip
// never repeats the row's own meaning (engine.RowSuppress).
func WithFreshRow(ctx context.Context, row string) context.Context {
	return context.WithValue(ctx, freshRowKey{}, row)
}

func FreshRowOf(ctx context.Context) string {
	s, _ := ctx.Value(freshRowKey{}).(string)
	return s
}

// CardFresh is the card's one freshness strip: the Library's dates when
// the title is in it, else the release date the list gave.
func CardFresh(ctx context.Context, store *database.Store, now time.Time, it metadata.DiscoverItem, titleID int64) (*engine.Fresh, error) {
	f := engine.FreshFacts{Kind: it.Kind, InLibrary: titleID != 0, Released: engine.ParseDay(it.Released)}
	if titleID != 0 {
		db := store.Read()
		var added, release string
		if err := db.QueryRowContext(ctx, "SELECT created_at, COALESCE(digital_date, cinema_date, '') FROM titles WHERE id = ?", titleID).Scan(&added, &release); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		f.Added = engine.ParseDay(added)
		if r := engine.ParseDay(release); !r.IsZero() {
			f.Released = r
		}
		if it.Kind == "show" {
			today := now.UTC().Format("2006-01-02")
			var last string
			if err := db.QueryRowContext(ctx, "SELECT COALESCE(max(air_date), '') FROM items WHERE title_id = ? AND kind = 'episode' AND air_date != '' AND substr(air_date, 1, 10) <= ?",
				titleID, today).Scan(&last); err != nil {
				return nil, err
			}
			f.LastEpisode = engine.ParseDay(last)
			rows, err := db.QueryContext(ctx, `SELECT p.number, COALESCE(min(i.air_date), p.release_date, '') FROM parts p LEFT JOIN items i ON i.part_id = p.id AND i.kind = 'episode' AND i.air_date != ''
				WHERE p.title_id = ? AND p.kind = 'season' AND p.number > 0 GROUP BY p.id ORDER BY p.number`, titleID)
			if err != nil {
				return nil, err
			}
			first := true
			for rows.Next() {
				var n int
				var start string
				if err := rows.Scan(&n, &start); err != nil {
					rows.Close()
					return nil, err
				}
				d := engine.ParseDay(start)
				if d.IsZero() {
					continue
				}
				if first {
					// The show's first air date stands in for its release.
					f.Released, first = d, false
				}
				if d.After(now) {
					if f.NextSeason.IsZero() {
						f.NextSeason, f.NextSeasonNumber = d, n
					}
				} else if n > 1 {
					f.SeasonStart = d
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
	}
	return engine.PickFreshIn(f, now, FreshRowOf(ctx)), nil
}

// MusicFresh is a music card's strip: New release on New from artists you
// have.
func MusicFresh(now time.Time, row string, c music.MusicCard) *engine.Fresh {
	f := engine.FreshFacts{Kind: c.Kind, Music: true}
	switch row {
	case "new_from":
		if !c.Upcoming {
			f.Released = engine.ParseDay(c.Date)
		}
	default:
		return nil
	}
	return engine.PickFreshIn(f, now, row)
}
