package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Lean music pages (music speed plan section 3): an artist is 2 MusicBrainz
// calls (the artist with url-rels, then one release-group browse of up to
// 100) and an album is 2 (the release group, then its first official
// release with recordings, for the tracklist). Older albums past the
// browse page are filled later at P4 (deep fill), never here.

// withMusicSecond makes the second call of an artist or album detail and
// merges it into source_data. A failed browse fails the artist (the queue
// retries it); a failed tracklist leaves the album without one.
func (c *Client) withMusicSecond(ctx context.Context, q model.Query, p *provider, r model.Result) (model.Result, error) {
	if q.Provider != "musicbrainz" || q.Operation != "detail" || (q.Kind != "artist" && q.Kind != "release-group") {
		return r, nil
	}
	var item map[string]any
	if json.Unmarshal(r.Data, &item) != nil {
		return r, nil
	}
	src, _ := item["source_data"].(map[string]any)
	if src == nil {
		return r, nil
	}
	id := strings.ToLower(q.ID)
	if q.Kind == "artist" {
		var page struct {
			Count  int   `json:"release-group-count"`
			Groups []any `json:"release-groups"`
		}
		if err := c.pacedGet(ctx, p, "/release-group", url.Values{"artist": {id}, "limit": {"100"}, "fmt": {"json"}}, &page); err != nil {
			return model.Result{}, err
		}
		if page.Groups == nil {
			page.Groups = []any{}
		}
		src["release-groups"] = page.Groups
		src["release-group-count"] = page.Count
	} else {
		rel := pickRelease(src["releases"])
		if rel == "" {
			return r, nil
		}
		var release struct {
			Media []any `json:"media"`
		}
		if c.pacedGet(ctx, p, "/release/"+rel, url.Values{"inc": {"recordings+artist-credits"}, "fmt": {"json"}}, &release) != nil || release.Media == nil {
			return r, nil
		}
		src["media"] = release.Media
		src["tracklist_release"] = rel
	}
	out, err := json.Marshal(item)
	if err != nil {
		return r, nil
	}
	r.Data = out
	return r, nil
}

// pickRelease is the earliest official release (any release if none is
// official) of a release group's release list.
func pickRelease(raw any) string {
	list, _ := raw.([]any)
	best, bestDate, bestOfficial := "", "", false
	for _, it := range list {
		m, _ := it.(map[string]any)
		id, _ := m["id"].(string)
		if !validMBID(id) {
			continue
		}
		date, _ := m["date"].(string)
		official := m["status"] == "Official"
		better := best == "" || (official && !bestOfficial) ||
			(official == bestOfficial && date != "" && (bestDate == "" || date < bestDate))
		if better {
			best, bestDate, bestOfficial = id, date, official
		}
	}
	return strings.ToLower(best)
}

// pacedGet is one MusicBrainz call outside requestURL, sharing the
// provider's gate and pacing slot with every other call.
func (c *Client) pacedGet(ctx context.Context, p *provider, path string, v url.Values, out any) error {
	if p.acquire(ctx) != nil {
		return &model.FetchError{Code: "upstream_timeout"}
	}
	defer func() { <-p.gate }()
	if p.reserve(ctx) != nil {
		return &model.FetchError{Code: "upstream_timeout"}
	}
	u := *p.base
	u.Path += path
	u.RawQuery = v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &model.FetchError{Code: "invalid_query"}
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return &model.FetchError{Code: "upstream_unavailable", Status: 503, RetryAfter: time.Second}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fe := &model.FetchError{Code: "upstream_status", Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
		if fe.RetryAfter > 0 {
			p.holdUntil(time.Now().Add(fe.RetryAfter))
		}
		return fe
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, c.opts.MaxBodyBytes+1))
	if err != nil || int64(len(b)) > c.opts.MaxBodyBytes || json.Unmarshal(b, out) != nil {
		return &model.FetchError{Code: "upstream_invalid"}
	}
	return nil
}

// MusicBrainzGet makes one MusicBrainz call (path such as "/release-group"
// or "/release/{mbid}", fmt=json added) through the provider's gate and
// pacing slot, shared with every other MusicBrainz call, and decodes the
// answer into out. A non-200 answer is a FetchError with its status.
func (c *Client) MusicBrainzGet(ctx context.Context, path string, v url.Values, out any) error {
	p, ok := c.providers["musicbrainz"]
	if !ok {
		return &model.FetchError{Code: "provider_unavailable"}
	}
	if strings.Contains(path, "..") || !strings.HasPrefix(path, "/") {
		return &model.FetchError{Code: "invalid_query"}
	}
	q := url.Values{}
	for k, vs := range v {
		q[k] = vs
	}
	q.Set("fmt", "json")
	return c.pacedGet(ctx, p, path, q, out)
}
