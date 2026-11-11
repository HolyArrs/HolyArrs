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

// MusicChartItem is one album or song of a region (or genre) chart, with
// the metadata server's popularity P (1 − (rank − 1)/L) and freshness F.
type MusicChartItem struct {
	Rank       int      `json:"rank"`
	ChartRank  int      `json:"chart_rank"`
	MBID       string   `json:"mbid"`
	ArtistMBID string   `json:"artist_mbid"`
	Title      string   `json:"title"`
	Artist     string   `json:"artist_name"`
	Date       string   `json:"release_date"`
	Explicit   bool     `json:"explicit"`
	Genres     []string `json:"genres"`
	Cover      string   `json:"cover"`
	P          float64  `json:"p"`
	F7         float64  `json:"f7"`
	F14        float64  `json:"f14"`
	F60        float64  `json:"f60"`
	// RankAgo is the rank 7 days ago (0: not charted then); nil until the
	// server keeps 7 days of ranks, so ΔR rows stay hidden.
	RankAgo *int `json:"rank_7d_ago,omitempty"`
}

// MusicChart is GET /v1/music/charts.
type MusicChart struct {
	Region string           `json:"region"`
	Kind   string           `json:"kind"`
	Genre  string           `json:"genre"`
	Items  []MusicChartItem `json:"items"`
}

// MusicGenre is one genre tile; an empty Art means the flat hue.
type MusicGenre struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Hue   int    `json:"hue"`
	Count int    `json:"count"`
	Art   string `json:"art"`
	// Arts is the slideshow (1 to 4 artist photos); Art is the first.
	Arts []string `json:"arts,omitempty"`
}

// MusicGenres is GET /v1/music/genres.
type MusicGenres struct {
	Region string       `json:"region"`
	Genres []MusicGenre `json:"genres"`
}

// musicGet reads one metadata server music answer into out.
func (s *Service) musicGet(ctx context.Context, path string, q url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return accessErr(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path+"?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
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
		return ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out) != nil {
		return ErrServiceInvalid
	}
	return nil
}

// MusicCharts reads a region's chart: kind albums or songs, genre a slug
// or empty.
func (s *Service) MusicCharts(ctx context.Context, region, kind, genre string) (MusicChart, error) {
	var out MusicChart
	q := url.Values{"region": {region}, "kind": {kind}}
	if genre != "" {
		q.Set("genre", genre)
	}
	if err := s.musicGet(ctx, "/v1/music/charts", q, &out); err != nil {
		return MusicChart{}, err
	}
	return out, nil
}

// MusicGenreList reads a region's genre tiles.
func (s *Service) MusicGenreList(ctx context.Context, region string) (MusicGenres, error) {
	var out MusicGenres
	if err := s.musicGet(ctx, "/v1/music/genres", url.Values{"region": {region}}, &out); err != nil {
		return MusicGenres{}, err
	}
	return out, nil
}

// MusicGenreTile reads a genre's tile picture (a grey JPEG, at most 40 KB)
// at the version tag the genre list named.
func (s *Service) MusicGenreTile(ctx context.Context, slug, version string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return nil, accessErr(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/music/genres/tile/"+url.PathEscape(slug)+".jpg?"+url.Values{"v": {version}}.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := s.doPending(ctx, req)
	if err != nil {
		return nil, unreached(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/jpeg"):
		return nil, ErrServiceUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	return b, nil
}

// MusicSuggestion is one row of the metadata server's suggest index
// (charted artists, albums and songs; no MusicBrainz call there).
type MusicSuggestion struct {
	Kind       string  `json:"kind"` // artist, album, song
	Name       string  `json:"name"`
	Artist     string  `json:"artist_name,omitempty"`
	MBID       string  `json:"mbid,omitempty"`
	ArtistMBID string  `json:"artist_mbid,omitempty"`
	Cover      string  `json:"cover,omitempty"`
	P          float64 `json:"p"`
}

// MusicSuggest reads GET /v1/music/suggest?q= (at most 10 rows). The
// caller's context bounds it; live typing gives it well under a second.
func (s *Service) MusicSuggest(ctx context.Context, q string) ([]MusicSuggestion, error) {
	var out struct {
		Items []MusicSuggestion `json:"items"`
	}
	if err := s.musicGet(ctx, "/v1/music/suggest", url.Values{"q": {q}}, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}
