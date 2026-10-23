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

// MusicCover reads an album's cover URL from the metadata server (Cover
// Art Archive, else Deezer, else iTunes by artist and title). ErrNotFound
// when no source has one.
func (s *Service) MusicCover(ctx context.Context, mbid, artist, title string) (string, error) {
	if !ValidMBID(mbid) {
		return "", ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return "", ErrServiceAccess
	}
	q := url.Values{}
	if a, t := strings.TrimSpace(artist), strings.TrimSpace(title); a != "" && t != "" && len(a) <= 300 && len(t) <= 300 {
		q.Set("artist", a)
		q.Set("title", t)
	}
	u := s.base + "/v1/music/cover/" + strings.ToLower(mbid)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.doPending(ctx, req)
	if err != nil {
		return "", ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return "", ErrServicePending
	case resp.StatusCode == http.StatusNotFound:
		return "", ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return "", ErrServiceUnavailable
	}
	var out struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out) != nil || !strings.HasPrefix(out.URL, "https://") {
		return "", ErrServiceInvalid
	}
	return out.URL, nil
}
