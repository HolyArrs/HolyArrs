package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ServerArt is the artwork the metadata server stores for a movie or
// show (title logo, banner, wide backgrounds). Every address is on the
// server's own picture store ({server}/v1/img/...); anything else is
// dropped.
type ServerArt struct {
	Logo        string   `json:"logo"`
	Banner      string   `json:"banner"`
	Thumb       string   `json:"thumb"`
	Backgrounds []string `json:"backgrounds"`
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

// storedPicture matches the rest of a picture address on the server's
// store: {kind}/{id}/{size}.{ext}, with an optional version.
var storedPicture = regexp.MustCompile(`^[a-z-]{2,20}/[A-Za-z0-9-]{1,40}/\d{2,4}\.(?:png|webp|jpg)(?:\?v=[A-Za-z0-9_-]{1,64})?$`)

// ownPicture keeps u when it is a picture on the server's own store.
func (s *Service) ownPicture(u string) string {
	if rest, ok := strings.CutPrefix(u, s.base+"/v1/img/"); ok && storedPicture.MatchString(rest) {
		return u
	}
	return ""
}

// Art reads the stored artwork of a movie or show (TMDB id). ErrNotFound
// when the server has none.
func (s *Service) Art(ctx context.Context, kind, id string) (ServerArt, error) {
	var out ServerArt
	if kind == "show" {
		kind = "tv"
	}
	if (kind != "movie" && kind != "tv") || !serviceID("tmdb", id) {
		return out, ErrServiceInvalid
	}
	if err := s.extra(ctx, "/v1/art/"+kind+"/"+id, &out); err != nil {
		return ServerArt{}, err
	}
	out.Logo, out.Banner, out.Thumb = s.ownPicture(out.Logo), s.ownPicture(out.Banner), s.ownPicture(out.Thumb)
	var keep []string
	for _, u := range out.Backgrounds {
		if u = s.ownPicture(u); u != "" {
			keep = append(keep, u)
		}
	}
	out.Backgrounds = keep
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
		return accessErr(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	aheadRequest(ctx, req)
	resp, err := s.doPending(ctx, req)
	if err != nil {
		return unreached(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return pendingFrom(b)
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
