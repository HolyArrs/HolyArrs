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

// ServiceTopItem is one ranked title of a service's list.
type ServiceTopItem struct {
	Rank       int    `json:"rank"`
	TMDBID     int64  `json:"tmdb_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path"`
}

// ServiceTopList is one streaming service's list: source is
// streaming-availability (that service's own top today) or tmdb-popular
// (TMDB's most popular on it, for Peacock, which the API does not cover).
type ServiceTopList struct {
	Service   string `json:"service"`
	Name      string `json:"name"`
	FetchedAt string `json:"fetched_at"`
	// AgeSeconds is the list's age when the server built the answer;
	// Stale is true after 14 days (still served).
	AgeSeconds int64            `json:"age_seconds"`
	Stale      bool             `json:"stale"`
	Source     string           `json:"source"`
	Items      []ServiceTopItem `json:"items"`
	// New are the titles newly added to the service in the last 14 days,
	// newest first (Streaming Availability services only).
	New []ServiceNewItem `json:"new,omitempty"`
}

// ServiceNewItem is one title newly added to a service.
type ServiceNewItem struct {
	TMDBID     int64  `json:"tmdb_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path"`
	AddedAt    string `json:"added_at"`
}

// ServiceTop is every service's list for one region.
type ServiceTop struct {
	Region      string           `json:"region"`
	Attribution string           `json:"attribution"`
	Services    []ServiceTopList `json:"services"`
}

// ServiceTop reads every service's top list for a region (ISO country).
// ErrNotFound when the server has none for it.
func (s *Service) ServiceTop(ctx context.Context, region string) (ServiceTop, error) {
	var out ServiceTop
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return out, accessErr(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/lists/service-top?"+url.Values{"region": {region}}.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return out, unreached(err)
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
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&out) != nil || len(out.Services) == 0 {
		return ServiceTop{}, ErrServiceInvalid
	}
	return out, nil
}
