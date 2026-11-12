package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// SimilarArtist is one artist like another (ListenBrainz listening
// sessions, through the metadata server), best first.
type SimilarArtist struct {
	MBID  string `json:"mbid"`
	Name  string `json:"name"`
	Score int64  `json:"score"`
}

// MusicSimilar reads the artists similar to one MusicBrainz artist.
// ErrNotFound when the server knows none.
func (s *Service) MusicSimilar(ctx context.Context, mbid string) ([]SimilarArtist, error) {
	if !ValidMBID(mbid) {
		return nil, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return nil, accessErr(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/music/similar/"+strings.ToLower(mbid), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.doPending(ctx, req)
	if err != nil {
		return nil, unreached(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return nil, ErrServicePending
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return nil, ErrServiceUnavailable
	}
	var out struct {
		Artists []SimilarArtist `json:"artists"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.Artists == nil {
		return nil, ErrServiceInvalid
	}
	list := out.Artists[:0]
	for _, a := range out.Artists {
		if ValidMBID(a.MBID) && strings.TrimSpace(a.Name) != "" {
			a.MBID = strings.ToLower(a.MBID)
			list = append(list, a)
		}
	}
	return list, nil
}
