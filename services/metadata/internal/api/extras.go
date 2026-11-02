package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
)

// ArtSource answers fanart.tv artwork (off without a key).
type ArtSource interface {
	Art(ctx context.Context, kind, id string) ([]byte, string, error)
	Status() extras.Status
}

// RatingsSource answers outside ratings (off without a key).
type RatingsSource interface {
	Ratings(ctx context.Context, kind, id string) ([]byte, string, error)
	Status() extras.Status
}

func numericID(raw string) bool {
	n, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == raw
}

// serveArt answers GET /v1/art/{movie|tv|music}/{id}: movie takes a TMDB
// id, tv a TMDB id (mapped to TVDB), music a MusicBrainz artist id.
func (s *Server) serveArt(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/art/"), "/")
	id = strings.ToLower(id)
	// Alias: older callers ask /v1/art/artist/{mbid}.
	if kind == "artist" {
		kind = "music"
	}
	valid := ok && len(r.URL.Query()) == 0 && ((kind == "movie" || kind == "tv") && numericID(id) || kind == "music" && mbidPattern.MatchString(id))
	if !valid {
		problem(w, 400, "invalid_query", 0)
		return
	}
	var src ArtSource
	if s.opts.Fanart != nil {
		src = s.opts.Fanart
	}
	s.serveExtra(w, src == nil, func() ([]byte, string, error) { return artNow(r.Context(), src, kind, id) }, "art:"+kind+":"+id, "art")
}

// artWait is how long a cold art request waits for its fetch (tests shorten it).
var artWait = 3 * time.Second

// errArtPending answers 202: the fetch is still running and will be kept.
var errArtPending = errors.New("art fetch still running")

// artNow fetches art right away (someone is waiting) but apart from the
// request: a slow fanart.tv answer is still kept when the asker stops
// waiting after artWait, so the next ask (Retry-After 1) gets it.
func artNow(ctx context.Context, src ArtSource, kind, id string) ([]byte, string, error) {
	type answer struct {
		body  []byte
		cache string
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		b, c, err := src.Art(fctx, kind, id)
		done <- answer{b, c, err}
	}()
	t := time.NewTimer(artWait)
	defer t.Stop()
	select {
	case a := <-done:
		return a.body, a.cache, a.err
	case <-t.C:
	case <-ctx.Done():
	}
	return nil, "pending", errArtPending
}

// serveRatings answers GET /v1/ratings/{movie|tv}/{tmdb_id}.
func (s *Server) serveRatings(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/ratings/"), "/")
	if !ok || len(r.URL.Query()) != 0 || (kind != "movie" && kind != "tv") || !numericID(id) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	var src RatingsSource
	if s.opts.Ratings != nil {
		src = s.opts.Ratings
	}
	s.serveExtra(w, src == nil, func() ([]byte, string, error) { return src.Ratings(r.Context(), kind, id) }, "ratings:"+kind+":"+id, "ratings")
}

func (s *Server) serveExtra(w http.ResponseWriter, off bool, load func() ([]byte, string, error), item, group string) {
	if off {
		problem(w, 404, "source_disabled", 0)
		return
	}
	body, cache, err := load()
	switch {
	case errors.Is(err, extras.ErrDisabled):
		problem(w, 404, "source_disabled", 0)
		return
	case errors.Is(err, errArtPending):
		w.Header().Set("X-Metadata-Cache", cache)
		w.Header().Set("Cache-Control", "no-store")
		problem(w, 202, "pending", time.Second)
		return
	case errors.Is(err, extras.ErrNotFound):
		w.Header().Set("X-Metadata-Cache", cache)
		problem(w, 404, "not_found", 0)
		return
	case err != nil:
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	if s.opts.Stats != nil && cache == "miss" {
		s.opts.Stats.Stored(item, group)
	}
	h := w.Header()
	h.Set("X-Metadata-Cache", cache)
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("Vary", "Authorization")
	h.Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}
