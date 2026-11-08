package api

import (
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// animeHandlers wires the anime answer to the part switches, the Kid gate
// and the details cache.
func (a *API) animeHandlers() *shows.AnimeHandlers {
	h := &shows.AnimeHandlers{
		Gate:    a.animeGate,
		Caching: func(w http.ResponseWriter, r *http.Request) { a.exploreCaching(w, r, "") },
		Details: func(kind, id string) *metadata.Details {
			if e, ok := a.discover.get("details|" + kind + "|" + id); ok {
				return e.details
			}
			return nil
		},
	}
	if a.s.AnimeMap != nil {
		h.Map = a.s.AnimeMap
	}
	return h
}

// metadataAnime serves the anime answer (kept for tests calling it direct).
func (a *API) metadataAnime(w http.ResponseWriter, r *http.Request) {
	a.animeHandlers().Anime(w, r)
}

// animeGate answers the part switch and the Kid gate, as on the details page:
// a title above the limit (or with no known rating) answers nothing, not even
// its other names.
func (a *API) animeGate(w http.ResponseWriter, r *http.Request, kind, id string) bool {
	if len(a.kindsOn(kind)) == 0 {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return false
	}
	if a.kidLimited(w, r) {
		return false
	}
	if max := a.kidMax(r); max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
			if !ok {
				webserver.Fail(w, http.StatusForbidden, aboveLimit)
				return false
			}
			key := "details|" + kind + "|" + id
			e, cached := a.discover.get(key)
			if !cached || time.Since(e.at) >= discoverTTL["details"] {
				det, err := d.Details(r.Context(), kind, id)
				if err != nil {
					webserver.Fail(w, http.StatusForbidden, aboveLimit)
					return false
				}
				e = discoverEntry{at: time.Now(), details: &det}
				a.discover.put(key, e)
			}
			rating = e.details.Rating
		}
		if rating == "" || !metadata.RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, aboveLimit)
			return false
		}
	}
	return true
}
