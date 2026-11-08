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

// MusicTrendItem is one ranked artist or album (a MusicBrainz release
// group) of a music trending list.
type MusicTrendItem struct {
	Rank       int    `json:"rank"`
	MBID       string `json:"mbid"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	Artist     string `json:"artist_name"`
	ArtistMBID string `json:"artist_mbid"`
	Listens    int64  `json:"listen_count"`
	// ReleaseDate and Tags come on the fresh and classic lists.
	ReleaseDate string   `json:"release_date"`
	Tags        []string `json:"tags"`
	// Cover (albums) and Image (artists) are the chart source's picture
	// links, when the metadata server has them.
	Cover string `json:"cover"`
	Image string `json:"image"`
}

// MusicTrend is one music trending list as the metadata server keeps it:
// ListenBrainz's weekly sitewide top for global, Deezer's charts for a
// country.
type MusicTrend struct {
	Kind    string           `json:"kind"`
	Region  string           `json:"region"`
	Source  string           `json:"source"`
	Regions []string         `json:"regions"`
	Items   []MusicTrendItem `json:"items"`
}

// MusicTrending reads one list: kind artist or album, region global or an
// ISO country. ErrNotFound when the server has no such list.
func (s *Service) MusicTrending(ctx context.Context, kind, region string) (MusicTrend, error) {
	var out MusicTrend
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return out, ErrServiceAccess
	}
	q := url.Values{"kind": {kind}, "region": {region}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/lists/music-trending?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.doPending(ctx, req)
	if err != nil {
		return out, ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return out, ErrServicePending
	case resp.StatusCode == http.StatusNotFound:
		return out, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return out, ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return out, ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return out, ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.Region == "" {
		return MusicTrend{}, ErrServiceInvalid
	}
	return out, nil
}
