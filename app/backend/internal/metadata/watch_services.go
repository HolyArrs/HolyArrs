package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// WatchService is one streaming service people can pick on Explore.
type WatchService struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Logo     string `json:"logo,omitempty"`
	Priority int    `json:"priority"`
}

// ServiceLister lists the streaming services TMDB knows for a region.
type ServiceLister interface {
	WatchServices(ctx context.Context, kind, region string) ([]WatchService, error)
}

// ServiceListerFor returns the service list surface selected for ctx.
func ServiceListerFor(ctx context.Context, provider Provider) (ServiceLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	l, ok := provider.(ServiceLister)
	return l, ok
}

// ServiceIDs says whether s is 1 to 20 service ids joined by |.
func ServiceIDs(s string) bool {
	parts := strings.Split(s, "|")
	if s == "" || len(parts) > 20 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 || n > 9999999 || strconv.FormatInt(n, 10) != p {
			return false
		}
	}
	return true
}

func isRegion(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}

// WatchServices reads TMDB's streaming services for one kind and region,
// best known first (TMDB's own order for that region).
func (t *TMDB) WatchServices(ctx context.Context, kind, region string) ([]WatchService, error) {
	if (kind != "movie" && kind != "show") || !isRegion(region) {
		return nil, fmt.Errorf("kind must be movie or show, region a country code")
	}
	b, err := t.get(ctx, "/watch/providers/"+tvPath(kind), url.Values{"watch_region": {region}, "language": {"en-US"}})
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []struct {
			ID         int64          `json:"provider_id"`
			Name       string         `json:"provider_name"`
			Logo       string         `json:"logo_path"`
			Priority   int            `json:"display_priority"`
			Priorities map[string]int `json:"display_priorities"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("TMDB: odd answer: %w", err)
	}
	out := []WatchService{}
	for _, r := range res.Results {
		if r.ID <= 0 || r.Name == "" {
			continue
		}
		s := WatchService{ID: r.ID, Name: r.Name, Priority: r.Priority}
		if p, ok := r.Priorities[region]; ok {
			s.Priority = p
		}
		if r.Logo != "" {
			s.Logo = t.images() + "/w92" + r.Logo
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out, nil
}

// WatchLister reads where one title streams, per country (TMDB watch
// providers, JustWatch data).
type WatchLister interface {
	WatchProviders(ctx context.Context, kind, tmdbID string) (map[string][]WatchService, error)
}

// WatchListerFor returns the watch providers surface selected for ctx.
func WatchListerFor(ctx context.Context, provider Provider) (WatchLister, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	l, ok := provider.(WatchLister)
	return l, ok
}

// WatchProviders reads a title's subscription services per country; an
// empty map when it streams nowhere.
func (t *TMDB) WatchProviders(ctx context.Context, kind, tmdbID string) (map[string][]WatchService, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 || (kind != "movie" && kind != "show") {
		return nil, ErrNotFound
	}
	b, err := t.get(ctx, "/"+tvPath(kind)+"/"+strconv.FormatInt(n, 10)+"/watch/providers", nil)
	if err != nil {
		return nil, err
	}
	out := t.streamingFrom(b)
	if out == nil {
		out = map[string][]WatchService{}
	}
	return out, nil
}

// streaming reads where a title streams (subscription services, per
// country); nil when it cannot (the page shows without it).
func (t *TMDB) streaming(ctx context.Context, kind string, id int64) map[string][]WatchService {
	b, err := t.get(ctx, "/"+tvPath(kind)+"/"+strconv.FormatInt(id, 10)+"/watch/providers", nil)
	if err != nil {
		return nil
	}
	return t.streamingFrom(b)
}

// streamingFrom reads a watch providers answer; nil when odd or empty.
func (t *TMDB) streamingFrom(b []byte) map[string][]WatchService {
	var res struct {
		Results map[string]struct {
			Flatrate []struct {
				ID       int64  `json:"provider_id"`
				Name     string `json:"provider_name"`
				Logo     string `json:"logo_path"`
				Priority int    `json:"display_priority"`
			} `json:"flatrate"`
		} `json:"results"`
	}
	if json.Unmarshal(b, &res) != nil {
		return nil
	}
	var out map[string][]WatchService
	for region, r := range res.Results {
		if !isRegion(region) {
			continue
		}
		var list []WatchService
		for _, p := range r.Flatrate {
			if p.ID <= 0 || p.Name == "" || len(list) == maxStreaming {
				continue
			}
			s := WatchService{ID: p.ID, Name: p.Name, Priority: p.Priority}
			if p.Logo != "" {
				s.Logo = t.images() + "/w92" + p.Logo
			}
			list = append(list, s)
		}
		if len(list) > 0 {
			if out == nil {
				out = map[string][]WatchService{}
			}
			sort.SliceStable(list, func(i, j int) bool { return list[i].Priority < list[j].Priority })
			out[region] = list
		}
	}
	return out
}
