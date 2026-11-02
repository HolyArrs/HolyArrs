package api

import (
	"net/http"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// NewMusicWindows answers the precomputed new music windows.
type NewMusicWindows interface {
	Window(window string) (body []byte, etag string, ok bool)
}

// serveMusicNew answers GET /v1/music/new?window=tracks|week|recent|upcoming
// straight from memory (ListenBrainz fresh releases merged with the Deezer
// editorial and charts, rebuilt daily).
func (s *Server) serveMusicNew(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	for k, values := range v {
		if len(values) != 1 || k != "window" {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	window := v.Get("window")
	if window == "" {
		window = musictrend.WindowWeek
	}
	valid := false
	for _, x := range musictrend.Windows {
		valid = valid || x == window
	}
	if !valid {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.chartsBuilding(w, r) {
		return
	}
	if s.opts.NewMusic == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, etag, ok := s.opts.NewMusic.Window(window)
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "private, max-age=21600")
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
