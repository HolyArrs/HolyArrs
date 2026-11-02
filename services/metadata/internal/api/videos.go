package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// HeroVideos keeps artist music-video IDs (internal/herovideo).
type HeroVideos interface {
	Get(ctx context.Context, mbid string) ([]byte, bool)
}

// serveVideos answers GET /v1/music/videos/{mbid}: the stored music-video
// IDs, or a partial answer while a P1 job collects them.
func (s *Server) serveVideos(w http.ResponseWriter, r *http.Request) {
	mbid := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/videos/"))
	if !mbidPattern.MatchString(mbid) || len(r.URL.Query()) != 0 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Videos == nil {
		problem(w, 404, "not_found", 0)
		return
	}
	if b, ok := s.opts.Videos.Get(r.Context(), mbid); ok {
		if !hasVideos(b) {
			w.Header().Set(partialHeader, "true")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(b)
		return
	}
	if s.opts.WantVideos != nil {
		s.opts.WantVideos(mbid)
	}
	w.Header().Set("Retry-After", "5")
	writeJSON(w, 202, map[string]any{"artist_mbid": mbid, "partial": true, "videos": []any{}})
}

// hasVideos reports whether a stored video answer lists at least one video.
func hasVideos(b []byte) bool {
	var v struct {
		Videos []json.RawMessage `json:"videos"`
	}
	return json.Unmarshal(b, &v) == nil && len(v.Videos) > 0
}

// videosPending reports whether the artist's video job has not stored an
// answer yet (the artist answer is then partial for the edge).
func (s *Server) videosPending(mbid string) bool {
	if s.opts.Videos == nil {
		return false
	}
	_, ok := s.opts.Videos.Get(context.Background(), strings.ToLower(mbid))
	return !ok
}

// withVideos adds the artist's stored music videos to an artist answer as
// data.videos (the app reads them there); unchanged when none are stored.
func (s *Server) withVideos(mbid string, data []byte) []byte {
	if s.opts.Videos == nil {
		return data
	}
	b, ok := s.opts.Videos.Get(context.Background(), strings.ToLower(mbid))
	if !ok {
		return data
	}
	var v struct {
		Videos json.RawMessage `json:"videos"`
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(b, &v) != nil || len(v.Videos) == 0 || json.Unmarshal(data, &obj) != nil || obj == nil {
		return data
	}
	obj["videos"] = v.Videos
	out, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}

// withPictures adds the stored picture records to an artist or album answer
// as data.pictures (version, size and main colour of each), so an install
// draws the right box and colour before the picture arrives; unchanged
// when none are stored.
func (s *Server) withPictures(kind, mbid string, data []byte) []byte {
	if s.opts.Pictures == nil {
		return data
	}
	pics := s.opts.Pictures(context.Background(), kind, strings.ToLower(mbid))
	var obj map[string]json.RawMessage
	if len(pics) == 0 || json.Unmarshal(data, &obj) != nil || obj == nil {
		return data
	}
	b, err := json.Marshal(pics)
	if err != nil {
		return data
	}
	obj["pictures"] = b
	out, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}
