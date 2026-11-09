package shows

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// animeHandlers wires the anime answer to the part switches, the Kid gate
// and the details cache.
func (h *Handlers) animeHandlers() *AnimeHandlers {
	return &AnimeHandlers{
		Gate:    h.animeGate,
		Caching: func(w http.ResponseWriter, r *http.Request) { h.Caching(w, r, "") },
		Details: h.CachedDetails,
		Map:     h.AnimeMap,
	}
}

// AnimeRoutes registers the anime answer on title pages.
func (h *Handlers) AnimeRoutes(m *http.ServeMux) { h.animeHandlers().Routes(m) }

// Anime serves the anime answer (for tests calling it direct).
func (h *Handlers) Anime(w http.ResponseWriter, r *http.Request) { h.animeHandlers().Anime(w, r) }

// animeGate answers the part switch and the Kid gate, as on the details page:
// a title above the limit (or with no known rating) answers nothing, not even
// its other names.
func (h *Handlers) animeGate(w http.ResponseWriter, r *http.Request, kind, id string) bool {
	if len(h.KindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return false
	}
	if h.KidLimited(w, r) {
		return false
	}
	if max := h.KidMax(r); max != "" {
		rating, inLibrary := h.LibraryRating(r.Context(), kind, id)
		if !inLibrary {
			d, ok := metadata.DiscovererFor(r.Context(), h.Metadata)
			if !ok {
				webserver.Fail(w, http.StatusForbidden, metadata.AboveLimit)
				return false
			}
			det, err := h.Details(r.Context(), d, kind, id)
			if err != nil {
				webserver.Fail(w, http.StatusForbidden, metadata.AboveLimit)
				return false
			}
			rating = det.Rating
		}
		if rating == "" || !metadata.RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, metadata.AboveLimit)
			return false
		}
	}
	return true
}
