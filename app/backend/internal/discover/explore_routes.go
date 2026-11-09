package discover

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// ExploreSwitch reads ?type=: all (default), movies or shows.
func ExploreSwitch(v string) (string, bool) {
	switch v {
	case "", "all":
		return "all", true
	case "movies", "shows":
		return v, true
	}
	return "", false
}

// ExploreParams reads the type switch shared by the head and the tail.
func ExploreParams(w http.ResponseWriter, r *http.Request) (sw string, ok bool) {
	sw, ok = ExploreSwitch(r.URL.Query().Get("type"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows")
	}
	return sw, ok
}

// ExploreViewKeyOK is the check accounts calls before it counts a view:
// the row types, title layers and strip categories Explore knows.
func ExploreViewKeyOK(kind, key string) bool {
	switch kind {
	case accounts.ViewRow:
		return engine.MinItems(key) != 0 || key == "hero"
	case accounts.ViewLayer:
		return ValidTitleLayer(key) || key == engine.LayerFixed
	case accounts.ViewCategory:
		return StripCategoryOK(key)
	}
	return false
}

// HeroCandidates are the titles the hero picks from (before its quality
// gate), best first; All takes 2 x HeroShuffleTop more (the tabs' share).
func HeroCandidates(pg engine.Page, all bool) []engine.Item {
	n := engine.HeroPool
	if all {
		n += 2 * engine.HeroShuffleTop
	}
	return engine.HeroRank(engine.HeroCandidates(pg.Head, pg.Leans, n), pg.Taste)
}

// ExploreTrailerTitles are the movies and shows a page shows first, each
// once: the hero's candidates, then the first 4 cards of the first 3 rows
// that have cards (the trailer pass hands it the hero's candidates only).
func ExploreTrailerTitles(hero []engine.Item, head []engine.Row) [][2]string {
	var out [][2]string
	seen := map[string]bool{}
	add := func(v engine.Item) {
		if (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" && !seen[v.Kind+v.TMDBID] {
			seen[v.Kind+v.TMDBID] = true
			out = append(out, [2]string{v.Kind, v.TMDBID})
		}
	}
	for _, v := range hero {
		add(v)
	}
	rows := 0
	for _, row := range head {
		if len(row.Items) == 0 || rows == 3 {
			continue
		}
		rows++
		for i, v := range row.Items {
			if i == 4 {
				break
			}
			add(v)
		}
	}
	return out
}
