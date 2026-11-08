package shows

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// AnimeView is a title page's anime answer: whether the title is anime,
// its AniList title variants (alternate titles) and, for shows, how each
// TMDB season maps onto the anime cours with its episode offsets.
type AnimeView struct {
	Anime bool `json:"anime"`
	// Mapped: the community ID lists know the title.
	Mapped   bool                   `json:"mapped"`
	AltNames []string               `json:"alt_names"`
	Titles   []metadata.AnimeTitle  `json:"titles"`
	Seasons  []metadata.AnimeSeason `json:"seasons"`
}

// AnimeHandlers serves the anime answer on title pages.
type AnimeHandlers struct {
	// Gate answers the part and Kid checks for kind/id, reporting false
	// when it already answered.
	Gate func(w http.ResponseWriter, r *http.Request, kind, id string) bool
	// Caching sets the Explore cache headers.
	Caching func(w http.ResponseWriter, r *http.Request)
	// Details is the details page already loaded for kind/id, if cached.
	Details func(kind, id string) *metadata.Details
	// Map is the metadata server's anime ID mapping, nil when unset.
	Map func(ctx context.Context, by, id string) (metadata.AnimeMap, error)
}

// Routes registers the anime route.
func (h *AnimeHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/anime", h.Anime)
}

// Anime answers GET /api/v1/metadata/{kind}/{tmdb_id}/anime. It uses
// the details the page already loaded (TMDB genre Animation and origin JP)
// and the metadata server's ID mapping; a mapping hit counts as anime too.
// No mapping or no server is not an error: the answer is just not mapped.
func (h *AnimeHandlers) Anime(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if !h.Gate(w, r, kind, id) {
		return
	}
	h.Caching(w, r)
	out := AnimeView{AltNames: []string{}, Titles: []metadata.AnimeTitle{}, Seasons: []metadata.AnimeSeason{}}
	name, counts := "", map[int]int{}
	if d := h.Details(kind, id); d != nil {
		name = d.Name
		out.Anime = metadata.LooksAnime(d.Genres, d.OriginCountry, d.OriginalLanguage)
		for _, s := range d.Seasons {
			counts[s.Number] = s.EpisodeCount
		}
	}
	if h.Map != nil {
		by := "tmdb_tv"
		if kind == "movie" {
			by = "tmdb_movie"
		}
		m, err := h.Map(r.Context(), by, id)
		switch {
		case err == nil && len(m.Entries) > 0:
			out.Anime, out.Mapped = true, true
			out.AltNames = m.AltNames(name)
			if m.Titles != nil {
				out.Titles = m.Titles
			}
			if kind == "show" {
				out.Seasons = m.Seasons(counts)
			}
		case err != nil && !errors.Is(err, metadata.ErrNotFound):
			w.Header().Set("Cache-Control", "no-store")
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}
