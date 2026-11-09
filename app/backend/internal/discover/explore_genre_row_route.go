package discover

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// exploreGenreRow answers GET /api/v1/explore/genre-row?type=all|movies|shows:
// the "Browse by genre" cards, picked from the member's assembled page
// (the same cached page the head and tail read, so no outside call). The
// page asks for it after the head, so it never slows the first row.
func (h *Handlers) exploreGenreRow(w http.ResponseWriter, r *http.Request) {
	sw, ok := ExploreParams(w, r)
	if !ok {
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	me := h.UserID(r)
	pg, _, err := h.ExplorePageFor(r.Context(), r, me, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// The same response-stage filters as the rows (exploreViews): a Kid's
	// rating limit and never-add titles, so no card shows a backdrop from a
	// title the person couldn't see in a row.
	if pg, err = h.genreRowPage(r.Context(), r, pg); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	cards := engine.BrowseGenres(pg, sw, pg.Taste, pg.Kid || h.KidMax(r) != "")
	h.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "genres": cards})
}

// genreRowPage drops the titles a row would not show this person.
func (h *Handlers) genreRowPage(ctx context.Context, r *http.Request, pg engine.Page) (engine.Page, error) {
	return GenreRowPage(ctx, pg, h.KidMax(r), h.KidItems, h.DropNeverAdd)
}

// searchBrowse answers GET /api/v1/explore/browse?type=all|movies|shows:
// the Browse sections and the Genres section's fast tiles, straight from
// the member's page (genres with at least BrowseMin titles that have a
// backdrop; TV's combined genres come split, as categories). The other
// sections load from /explore/browse/section as the grid scrolls.
func (h *Handlers) searchBrowse(w http.ResponseWriter, r *http.Request) {
	sw, ok := ExploreParams(w, r)
	if !ok {
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	_, tiles, _, err := h.browsePage(r.Context(), r, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "tiles": tiles, "sections": engine.BrowseSections})
}
