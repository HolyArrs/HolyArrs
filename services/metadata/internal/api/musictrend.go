package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// MusicLists answers the precomputed music trending lists.
type MusicLists interface {
	List(kind, region string) (body []byte, etag string, ok bool)
}

// serveMusicTrending answers GET /v1/lists/music-trending
// ?kind=artist|album&region=global|<ISO country> straight from memory.
func (s *Server) serveMusicTrending(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	for k, values := range v {
		if len(values) != 1 || (k != "kind" && k != "region") {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	kind, region := v.Get("kind"), strings.ToUpper(v.Get("region"))
	if region == "" || region == "GLOBAL" {
		region = "global"
	}
	extra := kind == musictrend.KindFresh || kind == musictrend.KindRising || kind == musictrend.KindClassic
	if extra && region != "global" {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if (kind != "artist" && kind != "album" && !extra) || (region != "global" && (len(region) != 2 || region[0] < 'A' || region[0] > 'Z' || region[1] < 'A' || region[1] > 'Z')) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.chartsBuilding(w, r) {
		return
	}
	if s.opts.Music == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, etag, ok := s.opts.Music.List(kind, region)
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

// SimilarArtists answers similar artists for one MusicBrainz artist id.
type SimilarArtists interface {
	Artists(ctx context.Context, mbid string) (body []byte, cache string, err error)
}

// serveMusicSimilar answers GET /v1/music/similar/{mbid}: artists like
// this one (Apple Music, else ListenBrainz listening sessions), cached by
// the source; 202 while Apple's names are still being matched.
func (s *Server) serveMusicSimilar(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/similar/"))
	if len(r.URL.Query()) > 0 || !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Similar == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	ask := s.opts.Similar.Artists
	if n, ok := s.opts.Similar.(interface {
		ArtistsNow(context.Context, string) ([]byte, string, error)
	}); ok {
		// A page open never waits on the source: a miss answers 202 and
		// is fetched in the background.
		ask = n.ArtistsNow
	}
	body, cache, err := ask(r.Context(), mbid)
	w.Header().Set("X-Metadata-Cache", cache)
	switch {
	case errors.Is(err, musictrend.ErrNoSimilar):
		problem(w, 404, "not_found", 0)
		return
	case errors.Is(err, musictrend.ErrPending):
		w.Header().Set("Retry-After", "5")
		writeJSON(w, 202, map[string]any{"mbid": mbid, "partial": true})
		return
	case err != nil:
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	if s.opts.Stats != nil && cache == "miss" {
		s.opts.Stats.Stored("similar:"+mbid, "similar")
	}
	h := w.Header()
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("Vary", "Authorization")
	h.Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}

var mbidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AlbumCovers answers one album's cover by release group.
type AlbumCovers interface {
	Album(ctx context.Context, mbid, artist, title string) (body []byte, cache string, err error)
}

var (
	// ErrNoCover: no source has a cover for the album.
	ErrNoCover = errors.New("no album cover")
	// ErrCoverLater: the cover is being fetched; ask again shortly.
	ErrCoverLater = errors.New("album cover pending")
)

// serveMusicCover answers GET /v1/music/cover/{mbid}?artist=&title=: the
// album's stored cover, as our own picture addresses (fetched at once when
// not stored; the names help an album the server has none for).
func (s *Server) serveMusicCover(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/cover/"))
	v := r.URL.Query()
	for k, values := range v {
		if len(values) != 1 || (k != "artist" && k != "title") || len(values[0]) > 300 {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	if !mbidPattern.MatchString(mbid) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Covers == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, cache, err := s.opts.Covers.Album(r.Context(), mbid, v.Get("artist"), v.Get("title"))
	w.Header().Set("X-Metadata-Cache", cache)
	switch {
	case errors.Is(err, ErrNoCover):
		problem(w, 404, "not_found", 0)
		return
	case errors.Is(err, ErrCoverLater):
		w.Header().Set("Retry-After", "5")
		writeJSON(w, 202, map[string]any{"mbid": mbid, "partial": true})
		return
	case err != nil:
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("Vary", "Authorization")
	h.Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}
