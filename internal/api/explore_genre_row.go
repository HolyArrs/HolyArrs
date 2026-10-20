package api

import (
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/explore"
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
	cards := explore.BrowseGenres(pg, sw, pg.Taste, pg.Kid || a.kidMax(r) != "")
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "genres": cards})
}
