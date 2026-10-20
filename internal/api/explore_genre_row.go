package api

import (
	"context"
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// exploreGenreRow answers GET /api/v1/explore/genre-row?type=all|movies|shows:
// the "Browse by genre" cards, picked from the member's assembled page
// (the same cached page the head and tail read, so no outside call). The
// page asks for it after the head, so it never slows the first row.
func (a *API) exploreGenreRow(w http.ResponseWriter, r *http.Request) {
	sw, topKind, topRegion, ok := exploreParams(w, r)
	if !ok {
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	me := a.person(r).UserID
	pg, _, _, err := a.explorePageFor(r.Context(), r, me, sw, topKind, topRegion)
	if err != nil {
		internal(w, err)
		return
	}
	// The same response-stage filters as the rows (exploreViews): a Kid's
	// rating limit and never-add titles, so no card shows a backdrop from a
	// title the person couldn't see in a row.
	if pg, err = a.genreRowPage(r.Context(), r, pg); err != nil {
		internal(w, err)
		return
	}
	cards := explore.BrowseGenres(pg, sw, pg.Taste, pg.Kid || a.kidMax(r) != "")
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "genres": cards})
}

// genreRowPage drops the titles a row would not show this person: above a
// Kid's rating limit, or marked never-add.
func (a *API) genreRowPage(ctx context.Context, r *http.Request, pg explore.Page) (explore.Page, error) {
	var all []metadata.DiscoverItem
	for _, rows := range [][]explore.Row{pg.Head, pg.Tail} {
		for _, row := range rows {
			for _, it := range row.Items {
				all = append(all, metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind})
			}
		}
	}
	if max := a.kidMax(r); max != "" {
		all = a.kidItems(ctx, max, all, 0)
	}
	all, err := a.dropNeverAdd(ctx, all)
	if err != nil {
		return pg, err
	}
	ok := make(map[string]bool, len(all))
	for _, it := range all {
		ok[it.Kind+":"+it.TMDBID] = true
	}
	return pg.Filter(func(it explore.Item) bool { return ok[it.Key()] }), nil
}

// searchBrowse answers GET /api/v1/explore/browse?type=all|movies|shows:
// Search's Browse tiles, every genre of the tab with its mood rows (the
// genre page's mood rows, metadata.SubMoods), from the same assembled page
// and the same filters as the genre row. A Kid gets only kid-safe ones.
func (a *API) searchBrowse(w http.ResponseWriter, r *http.Request) {
	sw, topKind, topRegion, ok := exploreParams(w, r)
	if !ok {
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	me := a.person(r).UserID
	pg, _, _, err := a.explorePageFor(r.Context(), r, me, sw, topKind, topRegion)
	if err != nil {
		internal(w, err)
		return
	}
	if pg, err = a.genreRowPage(r.Context(), r, pg); err != nil {
		internal(w, err)
		return
	}
	moods := func(genre string) []string {
		var out []string
		for _, m := range metadata.SubMoods(genre) {
			out = append(out, m.Label)
		}
		return out
	}
	tiles := explore.BrowseTiles(pg, sw, pg.Kid || a.kidMax(r) != "", moods)
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "tiles": tiles})
}
