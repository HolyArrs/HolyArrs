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
