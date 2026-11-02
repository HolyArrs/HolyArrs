package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musicdata"
)

// MusicDeezer answers the Deezer-derived marks this server keeps (14
// days), so an install never asks Deezer itself (musicdata.Service).
type MusicDeezer interface {
	DeezerArtist(ctx context.Context, mbid string) (musicdata.DeezerArtist, error)
	DeezerAlbum(ctx context.Context, id int64) (musicdata.DeezerAlbumInfo, error)
}

// serveMusicDeezer answers GET /v1/music/deezer/artist/{artist mbid} (the
// matched Deezer artist: its albums with explicit marks, fans, top songs)
// and GET /v1/music/deezer/album/{deezer album id} (label and per-track
// explicit marks). 404 when Deezer has no match; 503 with Retry-After
// when it failed and nothing is kept.
func (s *Server) serveMusicDeezer(w http.ResponseWriter, r *http.Request) {
	kind, id, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/music/deezer/"), "/")
	id = strings.ToLower(id)
	if len(r.URL.Query()) > 0 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	var n int64
	switch kind {
	case "artist":
		if !mbidPattern.MatchString(id) {
			problem(w, 400, "invalid_query", 0)
			return
		}
	case "album":
		var err error
		n, err = strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
			problem(w, 400, "invalid_query", 0)
			return
		}
	default:
		problem(w, 400, "invalid_query", 0)
		return
	}
	d, ok := s.opts.MusicData.(MusicDeezer)
	if !ok || s.opts.MusicData == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	var body any
	var err error
	if kind == "artist" {
		var a musicdata.DeezerArtist
		a, err = d.DeezerArtist(ctx, id)
		body = map[string]any{"schema_version": 1, "mbid": id, "source": "deezer", "artist": a}
	} else {
		var a musicdata.DeezerAlbumInfo
		a, err = d.DeezerAlbum(ctx, n)
		body = map[string]any{"schema_version": 1, "source": "deezer", "album": a}
	}
	switch {
	case errors.Is(err, musicdata.ErrNone):
		problem(w, 404, "not_found", 0)
		return
	case err != nil:
		problem(w, 503, "provider_unavailable", time.Minute)
		return
	}
	writeMusicJSON(w, r, body, 86400)
}
