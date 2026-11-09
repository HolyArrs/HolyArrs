package discover

import (
	"context"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// TrailerPassTitles are the movies and shows of every stored Explore pool
// (Kid pools left out) as each page shows them: every pool's hero
// candidates first, then every card of its head and tail rows in page order
// (Trending on streaming from its kept lists), each title once and none on
// the Never-add list.
func (h *Handlers) TrailerPassTitles(ctx context.Context) [][2]string {
	rows, err := h.Store.Read().QueryContext(ctx, "SELECT DISTINCT member_id, type_switch FROM explore_pool ORDER BY member_id, type_switch")
	if err != nil {
		return nil
	}
	var keys []PoolKey
	for rows.Next() {
		var k PoolKey
		if rows.Scan(&k.Member, &k.Switch) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	now := h.Explore.Now()
	day := engine.Day(now)
	var heroes [][2]string
	var cards []engine.Item
	for _, k := range keys {
		m, err := h.Explore.LoadMember(ctx, k.Member)
		if err != nil || m.Kid {
			continue
		}
		// Today's pool, else the newest older one, as Explore shows it.
		pool, _, err := h.Explore.ReadPool(ctx, k.Member, k.Switch, day, false)
		if err == nil && pool == nil {
			pool, _, err = h.Explore.ReadPool(ctx, k.Member, k.Switch, day, true)
		}
		if err != nil || pool == nil || pool.Kid {
			continue
		}
		pg := h.PoolPage(*pool, k.Member, k.Switch, m.Region, engine.Moment{Local: now.Local()})
		heroes = append(heroes, ExploreTrailerTitles(HeroCandidates(pg, k.Switch == "all"), nil)...)
		for _, row := range append(append([]engine.Row{}, pg.Head...), pg.Tail...) {
			if row.Type == engine.TypeTopServices {
				// The page fills this row itself, from the kept lists.
				cards = append(cards, h.ServiceTopCache.Kept(strings.ToUpper(m.Region), k.Switch, TopServicesMax)...)
				continue
			}
			cards = append(cards, row.Items...)
		}
	}
	items := make([]metadata.DiscoverItem, 0, len(heroes)+len(cards))
	for _, t := range heroes {
		items = append(items, metadata.DiscoverItem{Kind: t[0], TMDBID: t[1]})
	}
	for _, it := range cards {
		items = append(items, metadata.DiscoverItem{Kind: it.Kind, TMDBID: it.TMDBID})
	}
	items, err = h.DropNeverAdd(ctx, items)
	if err != nil {
		return nil
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, it := range items {
		t := [2]string{it.Kind, it.TMDBID}
		if (it.Kind == "movie" || it.Kind == "show") && it.TMDBID != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
