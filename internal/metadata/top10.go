package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Top10Item is one ranked title of a Netflix Top 10 list.
type Top10Item struct {
	Rank       int    `json:"rank"`
	TMDBID     int64  `json:"tmdb_id"`
	Title      string `json:"title"`
	Season     string `json:"season_title"`
	PosterPath string `json:"poster_path"`
	Views      int64  `json:"views"`
	Weeks      int    `json:"weeks_in_top10"`
}

// Top10 is one week's Netflix Top 10 list as the metadata server keeps it.
type Top10 struct {
	Week    string      `json:"week"`
	Regions []string    `json:"regions"`
	Items   []Top10Item `json:"items"`
}

// NetflixTop10 reads one list: kind movie or tv, region global or an ISO
// country, lang en or non-en (global only). ErrNotFound when the server
// has no such list.
func (s *Service) NetflixTop10(ctx context.Context, kind, region, lang string) (Top10, error) {
	var out Top10
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return out, ErrServiceAccess
	}
	q := url.Values{"kind": {kind}, "region": {region}}
	if lang != "" {
		q.Set("lang", lang)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/lists/netflix-top10?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return out, ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return out, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return out, ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return out, ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return out, ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.Week == "" {
		return Top10{}, ErrServiceInvalid
	}
	return out, nil
}

// ServerTrailer is the metadata server's trailer pick for a title.
type ServerTrailer struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	CheckedAt string `json:"checked_at"`
	Status    string `json:"status"`
}

// TrailerPick reads the server's chosen trailer (kind movie or show, lang
// two letters); the server skips keys it found broken. ErrNotFound when
// the title has none.
func (s *Service) TrailerPick(ctx context.Context, kind, id, lang string) (ServerTrailer, error) {
	var out ServerTrailer
	if kind == "show" {
		kind = "tv"
	}
	if (kind != "movie" && kind != "tv") || !serviceID("tmdb", id) || len(lang) != 2 {
		return out, ErrServiceInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return out, ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/trailers/"+kind+"/"+id+"?"+url.Values{"language": {lang}}.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return out, ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return out, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return out, ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return out, ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return out, ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out) != nil || !youTubeKey.MatchString(out.Key) {
		return ServerTrailer{}, ErrServiceInvalid
	}
	return out, nil
}
