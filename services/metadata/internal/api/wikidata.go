package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

// Wikidata answers awards, people facts, cross-walk IDs and Wikipedia
// summaries, plus the cached award-winner lists.
type Wikidata interface {
	Title(ctx context.Context, kind string, tmdb int64) (wikidata.Entity, error)
	Person(ctx context.Context, tmdb int64) (wikidata.Entity, error)
	TVmazePhoto(ctx context.Context, tmdb int64, name string) string
	Artist(ctx context.Context, mbid string) (wikidata.Entity, error)
	Winners(kind string) ([]wikidata.Winner, bool)
	Stats() wikidata.Stats
}

// serveWikidata answers GET /v1/wikidata/title/{movie|tv}/{tmdb_id},
// /v1/wikidata/person/{tmdb_id}, /v1/wikidata/artist/{mbid} and
// /v1/wikidata/award-winners?kind=movie|tv.
func (s *Server) serveWikidata(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/wikidata/"), "/")
	q := r.URL.Query()
	if parts[0] == "award-winners" {
		if len(parts) != 1 || len(q) != 1 || len(q["kind"]) != 1 || !wikidata.ValidKind(q.Get("kind")) {
			problem(w, 400, "invalid_query", 0)
			return
		}
	} else if parts[0] == "person" && len(q) == 1 && len(q["name"]) == 1 && len(q.Get("name")) <= 100 {
		// A person's name, for the TVmaze photo when Wikidata has none.
	} else if len(q) != 0 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Wikidata == nil {
		problem(w, 404, "wikidata_unavailable", 0)
		return
	}
	id := func(raw string) (int64, bool) {
		n, err := strconv.ParseInt(raw, 10, 64)
		return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == raw
	}
	var e wikidata.Entity
	var err error
	switch {
	case parts[0] == "award-winners":
		list, ok := s.opts.Wikidata.Winners(q.Get("kind"))
		if !ok {
			problem(w, 404, "list_unavailable", 0)
			return
		}
		w.Header().Set("X-Metadata-Cache", "hit")
		writeJSON(w, 200, map[string]any{"kind": q.Get("kind"), "results": list})
		return
	case parts[0] == "title" && len(parts) == 3 && wikidata.ValidKind(parts[1]):
		n, ok := id(parts[2])
		if !ok {
			problem(w, 400, "invalid_query", 0)
			return
		}
		e, err = s.opts.Wikidata.Title(r.Context(), parts[1], n)
	case parts[0] == "person" && len(parts) == 2:
		n, ok := id(parts[1])
		if !ok {
			problem(w, 400, "invalid_query", 0)
			return
		}
		e, err = s.opts.Wikidata.Person(r.Context(), n)
		// No photo on Wikidata (no entry, or Wikidata busy): TVmaze by name, when the app sent it.
		if name := q.Get("name"); name != "" && e.Image == "" {
			if img := s.opts.Wikidata.TVmazePhoto(r.Context(), n, name); img != "" {
				if err != nil {
					e = wikidata.Entity{Kind: "person", ID: parts[1], Awards: []wikidata.Award{}, Tallies: []wikidata.Tally{}}
					err = nil
				}
				e.Image = img
			}
		}
	case parts[0] == "artist" && len(parts) == 2 && wikidata.ValidMBID(parts[1]):
		e, err = s.opts.Wikidata.Artist(r.Context(), parts[1])
	default:
		problem(w, 400, "invalid_query", 0)
		return
	}
	if errors.Is(err, wikidata.ErrNotFound) {
		problem(w, 404, "not_found", 0)
		return
	}
	if err != nil {
		problem(w, 503, "lookup_failed", 5*time.Second)
		return
	}
	if parts[0] == "person" && s.opts.PersonPicture != nil {
		// A Commons photo is served from our picture store, so no
		// install contacts Wikimedia.
		if a := s.opts.PersonPicture(r.Context(), parts[1], e.Image); a != "" {
			e.Image = a
		}
	}
	writeJSON(w, 200, e)
}
