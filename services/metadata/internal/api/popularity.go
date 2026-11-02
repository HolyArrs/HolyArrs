package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/kworb"
)

// ArtistPopularity is the kworb reader (kworb.Service): kept figures
// only, never a read on ask.
type ArtistPopularity interface {
	Popularity(mbid string) *kworb.Popularity
	Opened(mbid string)
	Rising() ([]byte, bool)
}

// withPopularity adds the artist's kept kworb figures to an artist answer
// as data.popularity; left out when none are kept (the app shows nothing).
func (s *Server) withPopularity(mbid string, data []byte) []byte {
	if s.opts.Popularity == nil {
		return data
	}
	p := s.opts.Popularity.Popularity(strings.ToLower(mbid))
	if p == nil {
		return data
	}
	var obj map[string]json.RawMessage
	b, err := json.Marshal(p)
	if err != nil || json.Unmarshal(data, &obj) != nil || obj == nil {
		return data
	}
	obj["popularity"] = b
	out, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}

// serveMusicRising answers GET /v1/music/charts/rising: today's biggest
// gainers in monthly listeners that are matched to MusicBrainz (shared
// like the other chart rows).
func (s *Server) serveMusicRising(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) > 0 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Popularity == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	body, ok := s.opts.Popularity.Rising()
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	writeMusicJSON(w, r, json.RawMessage(body), 3600)
}

// popularityMax caps one popularity ask.
const popularityMax = 50

// serveMusicPopularity answers GET /v1/music/popularity?artists=a,b (up to
// 50 artist mbids): each artist's kept kworb figures, the same as an
// artist answer's data.popularity, null when none are kept. It is kept an
// hour at the edge, so the figures move daily while the artist answer
// (a day at the edge) stays put.
func (s *Server) serveMusicPopularity(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "artists") {
		return
	}
	ids, ok := idList(r.URL.Query().Get("artists"), popularityMax)
	if !ok {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.Popularity == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	type row struct {
		MBID       string            `json:"mbid"`
		Popularity *kworb.Popularity `json:"popularity"`
	}
	out := make([]row, 0, len(ids))
	asOf := ""
	for _, id := range ids {
		p := s.opts.Popularity.Popularity(id)
		if p != nil && p.AsOf > asOf {
			asOf = p.AsOf
		}
		out = append(out, row{MBID: id, Popularity: p})
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "source": "kworb", "as_of": asOf, "artists": out}, 900)
}
