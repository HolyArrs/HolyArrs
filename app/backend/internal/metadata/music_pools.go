package metadata

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotModified answers a conditional read whose copy is still current.
var ErrNotModified = errors.New("metadata: not modified")

// MusicPools reads GET /v1/music/pools, the music catalog bundle the row
// templates read. The request carries no query, so it is the same for
// every install. With etag set it is conditional: ErrNotModified means the
// copy held is current.
func (s *Service) MusicPools(ctx context.Context, etag string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return nil, "", ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/music/pools", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := s.doPending(ctx, req)
	if err != nil {
		return nil, "", ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return nil, etag, ErrNotModified
	case resp.StatusCode == http.StatusNotFound:
		return nil, "", ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, "", ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, "", ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return nil, "", ErrServiceUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 48<<20))
	if err != nil {
		return nil, "", ErrServiceUnavailable
	}
	return b, resp.Header.Get("ETag"), nil
}
