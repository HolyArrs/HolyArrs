package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// ServerArt is the metadata server's fanart.tv artwork for a title or
// artist. Every URL is on assets.fanart.tv; anything else is dropped.
type ServerArt struct {
	Logo        string   `json:"logo"`
	ClearArt    string   `json:"clearart"`
	Banner      string   `json:"banner"`
	Thumb       string   `json:"thumb"`
	Backgrounds []string `json:"backgrounds"`
	Thumbs      []string `json:"thumbs"`
}

// ServerScore is one outside rating from the metadata server.
type ServerScore struct {
	Source string  `json:"source"`
	Value  float64 `json:"value"`
	Max    float64 `json:"max"`
	Votes  int64   `json:"votes"`
}

// ServerRatings is a title's outside ratings (IMDb, Rotten Tomatoes,
// Metacritic, Letterboxd, Trakt).
type ServerRatings struct {
	IMDbID string        `json:"imdb_id"`
	Scores []ServerScore `json:"scores"`
}

const fanartAssets = "https://assets.fanart.tv/"

func fanartURL(u string) string {
	if strings.HasPrefix(u, fanartAssets) && !strings.ContainsAny(u, "\"'<> \r\n") && len(u) < 512 {
		return u
	}
	return ""
}

// Art reads fanart.tv artwork (kind movie or show: TMDB id; artist:
// MusicBrainz id). ErrNotFound when the server has none or the source is
// off.
func (s *Service) Art(ctx context.Context, kind, id string) (ServerArt, error) {
	var out ServerArt
	switch kind {
	case "show":
		kind = "tv"
	case "artist":
		kind = "music"
	}
	if (kind == "movie" || kind == "tv") && !serviceID("tmdb", id) || kind == "music" && !ValidMBID(id) || kind != "movie" && kind != "tv" && kind != "music" {
		return out, ErrServiceInvalid
	}
	if err := s.extra(ctx, "/v1/art/"+kind+"/"+id, &out); err != nil {
		return ServerArt{}, err
	}
	out.Logo, out.ClearArt, out.Banner, out.Thumb = fanartURL(out.Logo), fanartURL(out.ClearArt), fanartURL(out.Banner), fanartURL(out.Thumb)
	keep := func(l []string) []string {
		var k []string
		for _, u := range l {
			if u = fanartURL(u); u != "" {
				k = append(k, u)
			}
		}
		return k
	}
	out.Backgrounds, out.Thumbs = keep(out.Backgrounds), keep(out.Thumbs)
	return out, nil
}

// Ratings reads outside ratings for a TMDB movie or show. ErrNotFound when
// the server has none or the source is off.
func (s *Service) Ratings(ctx context.Context, kind, id string) (ServerRatings, error) {
	var out ServerRatings
	if kind == "show" {
		kind = "tv"
	}
	if (kind != "movie" && kind != "tv") || !serviceID("tmdb", id) {
		return out, ErrServiceInvalid
	}
	if err := s.extra(ctx, "/v1/ratings/"+kind+"/"+id, &out); err != nil {
		return ServerRatings{}, err
	}
	return out, nil
}

func (s *Service) extra(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if IsAhead(ctx) && s.hold.held(time.Now()) {
		return ErrServiceBusy
	}
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	aheadRequest(ctx, req)
	resp, err := s.http.Do(req)
	if err != nil {
		return ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		s.hold.set(resp.Header.Get("Retry-After"), nil, time.Now())
		return ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out) != nil {
		return ErrServiceInvalid
	}
	return nil
}
