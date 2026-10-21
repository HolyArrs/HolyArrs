package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/ratings"
)

// animeView is a title page's anime answer: whether the title is anime,
// its AniList title variants (alternate titles) and, for shows, how each
// TMDB season maps onto the anime cours with its episode offsets.
type animeView struct {
	Anime bool `json:"anime"`
	// Mapped: the community ID lists know the title.
	Mapped   bool                   `json:"mapped"`
	AltNames []string               `json:"alt_names"`
	Titles   []metadata.AnimeTitle  `json:"titles"`
	Seasons  []metadata.AnimeSeason `json:"seasons"`
}

// metadataAnime answers GET /api/v1/metadata/{kind}/{tmdb_id}/anime. It uses
// the details the page already loaded (TMDB genre Animation and origin JP)
// and the metadata server's ID mapping; a mapping hit counts as anime too.
// No mapping or no server is not an error: the answer is just not mapped.
func (a *API) metadataAnime(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
		fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusConflict, "that part is off")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	// The Kid gate, as on the details page: a title above the limit (or
	// with no known rating) answers nothing, not even its other names.
	if max := a.kidMax(r); max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
			if !ok {
				fail(w, http.StatusForbidden, aboveLimit)
				return
			}
			key := "details|" + kind + "|" + id
			e, cached := a.discover.get(key)
			if !cached || time.Since(e.at) >= discoverTTL["details"] {
				det, err := d.Details(r.Context(), kind, id)
				if err != nil {
					fail(w, http.StatusForbidden, aboveLimit)
					return
				}
				e = discoverEntry{at: time.Now(), details: &det}
				a.discover.put(key, e)
			}
			rating = e.details.Rating
		}
		if rating == "" || !ratings.Allows(max, kind, rating) {
			fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	a.exploreCaching(w, r, "")
	out := animeView{AltNames: []string{}, Titles: []metadata.AnimeTitle{}, Seasons: []metadata.AnimeSeason{}}
	name, counts := "", map[int]int{}
	if e, ok := a.discover.get("details|" + kind + "|" + id); ok && e.details != nil {
		d := e.details
		name = d.Name
		out.Anime = metadata.LooksAnime(d.Genres, d.OriginCountry, d.OriginalLanguage)
		for _, s := range d.Seasons {
			counts[s.Number] = s.EpisodeCount
		}
	}
	if a.s.AnimeMap != nil {
		by := "tmdb_tv"
		if kind == "movie" {
			by = "tmdb_movie"
		}
		m, err := a.s.AnimeMap(r.Context(), by, id)
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
	writeJSON(w, http.StatusOK, out)
}
