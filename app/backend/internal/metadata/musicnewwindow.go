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

// MusicNewItem is one new album (or, in the tracks window, one track and
// its album) from the metadata server's new music windows. MBID is always
// the album's MusicBrainz release group, so it can be asked for.
type MusicNewItem struct {
	Rank        int    `json:"rank"`
	MBID        string `json:"mbid"`
	ArtistMBID  string `json:"artist_mbid"`
	Title       string `json:"title"`
	Artist      string `json:"artist_name"`
	Track       string `json:"track"`
	ReleaseDate string `json:"release_date"`
	Type        string `json:"type"` // album, ep, single
	Cover       string `json:"cover"`
	Explicit    bool   `json:"explicit"`
	// Popularity is the Deezer chart or editorial position, 1 = top; 0 is unranked.
	Popularity int   `json:"popularity"`
	Listens    int64 `json:"listen_count"`
}

// MusicNew is one window: tracks, week, recent or upcoming.
type MusicNew struct {
	Window string         `json:"window"`
	Items  []MusicNewItem `json:"items"`
}

// MusicNewWindow reads one new music window. ErrNotFound when the server
// has none yet.
func (s *Service) MusicNewWindow(ctx context.Context, window string) (MusicNew, error) {
	var out MusicNew
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return out, ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/music/new?"+url.Values{"window": {window}}.Encode(), nil)
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
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.Window != window {
		return MusicNew{}, ErrServiceInvalid
	}
	return out, nil
}
