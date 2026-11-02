package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/trailers"
)

// Trailers picks each title's trailer and reports trailer health.
type Trailers interface {
	Pick(ctx context.Context, kind string, id int64, lang, who string) (trailers.Pick, error)
	Stats() trailers.Stats
}

// serveTrailer answers GET /v1/trailers/{movie|tv}/{tmdb_id}?language=xx:
// the title's chosen trailer, skipping keys found broken.
func (s *Server) serveTrailer(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/trailers/")
	kind, raw, ok := strings.Cut(rest, "/")
	id, err := strconv.ParseInt(raw, 10, 64)
	if !ok || !trailers.ValidKind(kind) || err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		problem(w, 400, "invalid_query", 0)
		return
	}
	v := r.URL.Query()
	for k, values := range v {
		if k != "language" || len(values) != 1 {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	lang := strings.ToLower(v.Get("language"))
	if lang == "" {
		lang = "en"
	}
	if len(lang) != 2 || lang[0] < 'a' || lang[0] > 'z' || lang[1] < 'a' || lang[1] > 'z' {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Trailers == nil {
		problem(w, 404, "trailers_unavailable", 0)
		return
	}
	p, err := s.opts.Trailers.Pick(r.Context(), kind, id, lang, asker(r))
	if errors.Is(err, trailers.ErrNoTrailer) {
		problem(w, 404, "no_trailer", 0)
		return
	}
	if err != nil {
		problem(w, 503, "lookup_failed", 5*time.Second)
		return
	}
	w.Header().Set("X-Metadata-Cache", "hit")
	writeJSON(w, 200, p)
}
