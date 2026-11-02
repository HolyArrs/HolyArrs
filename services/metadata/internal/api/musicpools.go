package api

import "net/http"

// MusicPools is the music catalog bundle job: one JSON document with every
// catalog album and artist and each Browse key's members.
type MusicPools interface {
	Pools() (body []byte, etag string, ok bool)
}

// serveMusicPools answers GET /v1/music/pools. It takes no parameters, so
// every install asks the same question and gets the same answer.
func (s *Server) serveMusicPools(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r) {
		return
	}
	if s.opts.Pools == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, etag, ok := s.opts.Pools.Pools()
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
