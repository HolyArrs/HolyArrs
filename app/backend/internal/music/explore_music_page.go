package music

import (
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// exploreMusic answers GET /api/v1/explore/music?genre=<tag>&region=<code>.
func (h *Handlers) exploreMusic(w http.ResponseWriter, r *http.Request) {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) {
		webserver.Fail(w, http.StatusNotFound, "music is turned off")
		return
	}
	// Music has no ratings, so a Kid limit shows none of it.
	if h.KidMax(r) != "" {
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"kid": true, "cold": false, "genres": []string{}, "genre": "", "hero": nil, "rows": []MusicRow{}})
		return
	}
	if !h.Limit(h.Person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	h.exploreMusicPage(w, r)
}

type nameKey struct{ kind, mbid, name, artist, cover string }

// musicNamesMax caps the listed albums and artists (the list starts over
// past it); room for a household's open pages, catalog items aside (the
// picture route finds those in the catalog).
const musicNamesMax = 60000

// rememberMusicNames lets the picture route serve these albums and artists.
func (h *Handlers) rememberMusicNames(list []nameKey) {
	h.rememberNames(list)
	// Every row and page that lists them also gets their pictures stored.
	h.warmArtItems(list)
}
