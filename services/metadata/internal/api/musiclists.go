package api

import (
	"net/http"
	"strings"
)

// MusicShelves is the world-lists job: one region's shelves of albums and
// artists (MusicBrainz ids only) with their chart signals.
type MusicShelves interface {
	Shelves(region string) (body []byte, etag string, ok bool)
}

// serveMusicShelves answers GET /v1/music/lists/shelves?region=XX (global
// when absent; a region without its own chart gets the global shelves).
func (s *Server) serveMusicShelves(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "region") {
		return
	}
	region := strings.ToLower(r.URL.Query().Get("region"))
	if region == "" {
		region = "global"
	}
	if !regionRe.MatchString(region) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.chartsBuilding(w, r) {
		return
	}
	if s.opts.Shelves == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, etag, ok := s.opts.Shelves.Shelves(region)
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "private, max-age=3600")
	h.Set("Vary", "Authorization")
	h.Set("X-Metadata-Cache", "hit")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}
