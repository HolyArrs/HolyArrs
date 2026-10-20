package api

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

type top10RanksKey struct{}

type freshRowKey struct{}

// withFreshRow names the row type the cards are joined for, so their strip
// never repeats the row's own meaning (explore.RowSuppress).
func withFreshRow(ctx context.Context, row string) context.Context {
	return context.WithValue(ctx, freshRowKey{}, row)
}

func freshRowOf(ctx context.Context) string {
	s, _ := ctx.Value(freshRowKey{}).(string)
	return s
}

// withTop10Ranks carries this week's Top 10 ranks (kind:tmdb -> rank) for
// the member's region, so cards in other rows can show the Top 10 tag.
func withTop10Ranks(ctx context.Context, ranks map[string]int) context.Context {
	return context.WithValue(ctx, top10RanksKey{}, ranks)
}

func top10RankOf(ctx context.Context, key string) int {
	m, _ := ctx.Value(top10RanksKey{}).(map[string]int)
	return m[key]
}

// top10Ranks reads the ranks from a Top 10 answer body.
func top10Ranks(body map[string]any) map[string]int {
	out := map[string]int{}
	items, _ := body["items"].([]top10View)
	for _, v := range items {
		k := v.Kind + ":" + v.TMDBID
		if v.TMDBID != "" && (out[k] == 0 || v.Rank < out[k]) {
			out[k] = v.Rank
		}
	}
	return out
}

// cardFresh is the card's one freshness strip: the Library's dates when
// the title is in it, else the release date the list gave.
func (a *API) cardFresh(ctx context.Context, it metadata.DiscoverItem, titleID int64) (*explore.Fresh, error) {
	f := explore.FreshFacts{Kind: it.Kind, InLibrary: titleID != 0, Released: explore.ParseDay(it.Released), Top10Rank: top10RankOf(ctx, it.Kind+":"+it.TMDBID)}
	now := a.exploreNow()
	if titleID != 0 {
		db := a.s.Store.Read()
		var added, release string
		if err := db.QueryRowContext(ctx, "SELECT created_at, COALESCE(digital_date, cinema_date, '') FROM titles WHERE id = ?", titleID).Scan(&added, &release); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		f.Added = explore.ParseDay(added)
		if r := explore.ParseDay(release); !r.IsZero() {
			f.Released = r
		}
		if it.Kind == "show" {
			today := now.UTC().Format("2006-01-02")
			var last string
			if err := db.QueryRowContext(ctx, "SELECT COALESCE(max(air_date), '') FROM items WHERE title_id = ? AND kind = 'episode' AND air_date != '' AND substr(air_date, 1, 10) <= ?",
				titleID, today).Scan(&last); err != nil {
				return nil, err
			}
			f.LastEpisode = explore.ParseDay(last)
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
				d := explore.ParseDay(start)
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
	return explore.PickFreshIn(f, now, freshRowOf(ctx)), nil
}

// musicFresh is a music card's strip: New release on New from artists you
// have.
func (a *API) musicFresh(row string, c MusicCard) *explore.Fresh {
	f := explore.FreshFacts{Kind: c.Kind, Music: true}
	switch row {
	case "new_from":
		if !c.Upcoming {
			f.Released = explore.ParseDay(c.Date)
		}
	default:
		return nil
	}
	return explore.PickFreshIn(f, a.exploreNow(), row)
}
