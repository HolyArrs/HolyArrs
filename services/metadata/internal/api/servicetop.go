package api

import (
	"net/http"
	"strings"
)

// ServiceTop answers the precomputed per-service top lists.
type ServiceTop interface {
	List(region string) (body []byte, etag string, ok bool)
	// Stats is each service's state for /v1/stats.
	Stats() any
}

// serveServiceTop answers GET /v1/lists/service-top?region=<ISO country>
// (default US) straight from memory: every service in one response.
func (s *Server) serveServiceTop(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	for k, values := range v {
		if len(values) != 1 || k != "region" {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	region := strings.ToUpper(v.Get("region"))
	if region == "" {
		region = "US"
	}
	if len(region) != 2 || region[0] < 'A' || region[0] > 'Z' || region[1] < 'A' || region[1] > 'Z' {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.ServiceTop == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, etag, ok := s.opts.ServiceTop.List(region)
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
