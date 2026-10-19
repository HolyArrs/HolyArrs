package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DeezerBase is Deezer's public API, which answers artist searches with
// pictures and needs no key.
const DeezerBase = "https://api.deezer.com"

// ArtistImages finds an artist's picture by name in Deezer's public artist
// search. It is optional: a nil *ArtistImages finds nothing. Answers are
// kept for TTL (24 hours by default), misses too.
type ArtistImages struct {
	BaseURL string // default DeezerBase
	HTTP    *http.Client
	Timeout time.Duration
	Gap     time.Duration // least time between requests, default 250ms
	TTL     time.Duration

	once sync.Once
	g    *getter
}

func (d *ArtistImages) getter() *getter {
	d.once.Do(func() {
		ttl := d.TTL
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		d.g = &getter{name: "Deezer", http: d.HTTP, timeout: d.Timeout, gap: d.Gap, ttl: ttl}
	})
	return d.g
}

func (d *ArtistImages) base() string {
	if d.BaseURL == "" {
		return DeezerBase
	}
	return strings.TrimRight(d.BaseURL, "/")
}

// Image is the picture URL of the artist with this name (case does not
// matter), or "" when none is found or the artist has no picture.
func (d *ArtistImages) Image(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if d == nil || name == "" {
		return "", nil
	}
	u := d.base() + "/search/artist?" + url.Values{"q": {name}, "limit": {"10"}}.Encode()
	b, err := d.getter().do(ctx, []string{d.base()}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	if err != nil {
		return "", fmt.Errorf("Deezer artist search: %w", err)
	}
	var out struct {
		Data []struct {
			Name   string `json:"name"`
			XL     string `json:"picture_xl"`
			Big    string `json:"picture_big"`
			Medium string `json:"picture_medium"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("Deezer artist search: %w", err)
	}
	for _, a := range out.Data {
		if !strings.EqualFold(strings.TrimSpace(a.Name), name) {
			continue
		}
		for _, p := range []string{a.XL, a.Big, a.Medium} {
			// An artist without a picture has an empty picture id in the path.
			if p != "" && !strings.Contains(p, "/artist//") {
				return p, nil
			}
		}
		return "", nil
	}
	return "", nil
}
