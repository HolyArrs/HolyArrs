package discover

import (
	"context"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// awardWindows spread the Award winners lists over the decades, so the
// collection is not only this year's films.
var awardWindows = [][2]string{{"", ""}, {"1960-01-01", "1989-12-31"}, {"1990-01-01", "2004-12-31"}, {"2005-01-01", "2014-12-31"}, {"2015-01-01", ""}}

// AwardItems builds the Award winners collection: well-known titles per
// decade (and the best rated), kept when Wikidata lists a major award
// win for them. ok is false when the winner lists are not available, so
// the caller keeps the keyword lists.
func AwardItems(ctx context.Context, list func(context.Context, engine.Request) ([]engine.Item, error), winners func(ctx context.Context, kind string) (map[string]bool, bool), c engine.Category, sw string, kid bool) ([]engine.Item, bool, error) {
	var lists [][]engine.Item
	found := false
	for _, base := range c.Requests(sw, kid) {
		ids, ok := winners(ctx, base.Kind)
		if !ok {
			continue
		}
		found = true
		base.Keywords, base.MinVotes = "", 300
		var reqs []engine.Request
		for _, win := range awardWindows {
			r := base
			r.From, r.To = win[0], win[1]
			reqs = append(reqs, r)
		}
		best := base
		best.Sort, best.MinVotes = "rating", 2000
		reqs = append(reqs, best)
		for _, q := range reqs {
			items, err := list(ctx, q)
			if err != nil {
				return nil, true, err
			}
			kept := []engine.Item{}
			for _, it := range items {
				if ids[it.TMDBID] {
					kept = append(kept, it)
				}
			}
			lists = append(lists, kept)
		}
	}
	if !found {
		return nil, false, nil
	}
	return engine.MergeTop(lists, 80), true, nil
}

// awardItems builds the Award winners collection from src.
func (h *Handlers) awardItems(ctx context.Context, src ExploreSource, c engine.Category, sw string, kid bool) ([]engine.Item, bool, error) {
	return AwardItems(ctx, src.List, h.AwardWinners, c, sw, kid)
}
