package api

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/explore"
)

// awardWindows spread the Award winners lists over the decades, so the
// collection is not only this year's films.
var awardWindows = [][2]string{{"", ""}, {"1960-01-01", "1989-12-31"}, {"1990-01-01", "2004-12-31"}, {"2005-01-01", "2014-12-31"}, {"2015-01-01", ""}}

// awardItems builds the Award winners collection: well-known titles per
// decade (and the best rated), kept when Wikidata lists a major award
// win for them. ok is false when the winner lists are not available, so
// the caller keeps the keyword lists.
func (a *API) awardItems(ctx context.Context, src exploreSource, c explore.Category, sw string, kid bool) ([]explore.Item, bool, error) {
	var lists [][]explore.Item
	found := false
	for _, base := range c.Requests(sw, kid) {
		ids, ok := a.titles.AwardWinners(ctx, base.Kind)
		if !ok {
			continue
		}
		found = true
		base.Keywords, base.MinVotes = "", 300
		var reqs []explore.Request
		for _, win := range awardWindows {
			r := base
			r.From, r.To = win[0], win[1]
			reqs = append(reqs, r)
		}
		best := base
		best.Sort, best.MinVotes = "rating", 2000
		reqs = append(reqs, best)
		for _, q := range reqs {
			items, err := src.List(ctx, q)
			if err != nil {
				return nil, true, err
			}
			kept := []explore.Item{}
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
	return explore.MergeTop(lists, 80), true, nil
}
