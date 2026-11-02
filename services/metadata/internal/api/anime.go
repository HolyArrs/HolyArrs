package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/anime"
)

// AnimeMap answers anime ID mappings from the daily community lists.
type AnimeMap interface {
	Lookup(ctx context.Context, kind string, id int64) (anime.Answer, error)
}

// serveAnimeMap answers GET /v1/anime/map with exactly one of
// tmdb= (plus kind=tv|movie, default tv), tvdb=, anilist= or mal=.
func (s *Server) serveAnimeMap(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	kind, raw, kindParam := "", "", "tv"
	for k, values := range v {
		if len(values) != 1 {
			problem(w, 400, "invalid_query", 0)
			return
		}
		switch k {
		case "tmdb", "tvdb", "anilist", "mal":
			if kind != "" {
				problem(w, 400, "invalid_query", 0)
				return
			}
			kind, raw = k, values[0]
		case "kind":
			kindParam = values[0]
		default:
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if kind == "" || err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw || (kindParam != "tv" && kindParam != "movie") || (kindParam == "movie" && kind != "tmdb") {
		problem(w, 400, "invalid_query", 0)
		return
	}
	key := map[string]string{"tvdb": anime.KeyTVDB, "anilist": anime.KeyAniList, "mal": anime.KeyMAL}[kind]
	if kind == "tmdb" {
		key = anime.KeyTMDBTV
		if kindParam == "movie" {
			key = anime.KeyTMDBMovie
		}
	}
	if s.opts.Anime == nil {
		problem(w, 404, "anime_unavailable", 0)
		return
	}
	a, err := s.opts.Anime.Lookup(r.Context(), key, id)
	switch {
	case errors.Is(err, anime.ErrNotFound):
		problem(w, 404, "not_anime", 0)
	case errors.Is(err, anime.ErrNoData):
		problem(w, 503, "anime_loading", time.Minute)
	case err != nil:
		problem(w, 503, "lookup_failed", 5*time.Second)
	default:
		w.Header().Set("X-Metadata-Cache", "hit")
		writeJSON(w, 200, a)
	}
}
