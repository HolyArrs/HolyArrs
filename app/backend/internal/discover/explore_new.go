package discover

import (
	"strconv"
	"time"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// WithoutHero drops the hero's titles from rows.
func WithoutHero(rows []engine.Row, hero map[string]bool) []engine.Row {
	if len(hero) == 0 {
		return rows
	}
	out := make([]engine.Row, 0, len(rows))
	for _, r := range rows {
		items := make([]engine.Item, 0, len(r.Items))
		for _, it := range r.Items {
			if !hero[it.Key()] {
				items = append(items, it)
			}
		}
		r.Items = items
		out = append(out, r)
	}
	return out
}

// NewFresh is the card strip a title's newness gives (Phase C): Added 2d
// ago, Out Nov 4, Season 3, New episode Fri, New series.
func NewFresh(f engine.NewFact, now time.Time) *engine.Fresh {
	if f.At.IsZero() {
		return nil
	}
	days := int(now.Sub(f.At).Hours() / 24)
	switch f.Kind {
	case engine.NewService:
		t := "Added today"
		if days == 1 {
			t = "Added yesterday"
		} else if days > 1 {
			t = "Added " + strconv.Itoa(days) + "d ago"
		}
		return &engine.Fresh{Kind: engine.FreshRecentlyAdded, Text: t}
	case engine.NewRelease:
		return &engine.Fresh{Kind: engine.FreshNewRelease, Text: "Out " + f.At.Format("Jan 2")}
	case engine.NewUpcoming:
		return &engine.Fresh{Kind: engine.FreshComing, Text: "Out " + f.At.Format("Jan 2")}
	case engine.NewSeason:
		return &engine.Fresh{Kind: engine.FreshNewSeason, Text: "Season " + strconv.Itoa(f.Season)}
	case engine.NewEpisode:
		if days <= 6 {
			return &engine.Fresh{Kind: engine.FreshNewEpisode, Text: "New episode " + f.At.Format("Mon")}
		}
		return &engine.Fresh{Kind: engine.FreshNewEpisode, Text: "New episodes"}
	case engine.NewSeries:
		return &engine.Fresh{Kind: engine.FreshNewRelease, Text: "New series"}
	}
	return nil
}
