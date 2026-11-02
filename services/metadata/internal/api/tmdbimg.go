package api

import (
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// TMDB pictures stay links: installs fetch them from TMDB's own network.
// When that network fails an install asks /v1/img/tmdb/{size}/{file}, and
// the server fetches the picture from TMDB and passes it straight through
// (nothing is stored here; TMDB names a file by its content, so the edge
// keeps the answer for a year).

// TMDBImageBase is where TMDB serves pictures.
const TMDBImageBase = "https://image.tmdb.org/t/p"

// tmdbImageSizes are the sizes TMDB serves.
var tmdbImageSizes = map[string]bool{"w92": true, "w154": true, "w185": true, "w300": true, "w342": true, "w500": true, "w780": true, "w1280": true, "h632": true, "original": true}

var tmdbImageFile = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}\.(jpg|png|svg|webp)$`)

// maxTMDBPicture is the largest picture passed through.
const maxTMDBPicture = 20 << 20

// tmdbImageTypes are the only answers passed through; anything else
// (text/html included) is refused.
var tmdbImageTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/svg+xml": true}

// svgPolicy keeps an SVG served from our origin from running anything if
// it is opened on its own (TMDB's logos are often SVG).
const svgPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

func (s *Server) serveTMDBImage(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) != 3 || !tmdbImageSizes[parts[1]] || !tmdbImageFile.MatchString(parts[2]) || len(r.URL.Query()) != 0 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	base := s.opts.TMDBImages
	if base == "" {
		base = TMDBImageBase
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base+"/"+parts[1]+"/"+parts[2], nil)
	if err != nil {
		problem(w, 400, "invalid_query", 0)
		return
	}
	client := s.opts.TMDBImageClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		problem(w, 503, "provider_unavailable", 5*time.Second)
		return
	}
	defer resp.Body.Close()
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch {
	case resp.StatusCode == http.StatusNotFound:
		problem(w, 404, "not_found", 0)
		return
	case resp.StatusCode != 200 || resp.ContentLength > maxTMDBPicture:
		problem(w, 503, "provider_unavailable", 5*time.Second)
		return
	case !tmdbImageTypes[ct]:
		problem(w, 502, "not_a_picture", 0)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("X-Content-Type-Options", "nosniff")
	if ct == "image/svg+xml" {
		h.Set("Content-Security-Policy", svgPolicy)
	}
	if resp.ContentLength > 0 {
		h.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(200)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxTMDBPicture))
}
