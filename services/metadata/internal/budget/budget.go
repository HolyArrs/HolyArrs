// Package budget caps upstream calls per provider (TMDB, MusicBrainz,
// ListenBrainz, Deezer, Wikidata, Wikipedia) for the whole server, whatever the number of
// credentials. A spent budget fails the call with code "upstream_budget",
// so callers serve their cache (stale included) instead.
package budget

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
)

// DefaultPerHour is the default budget per provider.
var DefaultPerHour = map[string]int{"tmdb": 20000, "musicbrainz": 3000, "listenbrainz": 2000, "deezer": 6000, "anilist": 1200, "fanart": 2000, "mdblist": 25, "wikidata": 3000, "wikipedia": 3000}

// mdblist: 25 an hour is at most 600 a day, well under the free tier's
// 1000 a day; when spent, callers serve their cache or nothing.

// ErrSpent is returned (wrapped in a FetchError for Fetch) when a budget is used up.
var ErrSpent = errors.New("upstream_budget")

// Budget is one token bucket per provider, refilled evenly over the hour.
type Budget struct {
	mu      sync.Mutex
	perHour map[string]float64
	tokens  map[string]float64
	updated map[string]time.Time
	spent   map[string]int64
	Now     func() time.Time
}

// New builds a budget; perHour overrides DefaultPerHour per provider.
func New(perHour map[string]int) *Budget {
	b := &Budget{perHour: map[string]float64{}, tokens: map[string]float64{}, updated: map[string]time.Time{}, spent: map[string]int64{}}
	for k, v := range DefaultPerHour {
		b.perHour[k] = float64(v)
	}
	for k, v := range perHour {
		if v > 0 {
			b.perHour[k] = float64(v)
		}
	}
	return b
}

func (b *Budget) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Take spends one call for provider; false when the budget is spent.
// Providers without a budget always pass.
func (b *Budget) Take(provider string) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	rate, ok := b.perHour[provider]
	if !ok {
		return true
	}
	now := b.now()
	t, seen := b.tokens[provider]
	if !seen {
		t = rate
	} else {
		t = min(rate, t+now.Sub(b.updated[provider]).Hours()*rate)
	}
	b.updated[provider] = now
	if t < 1 {
		b.tokens[provider] = t
		b.spent[provider]++
		return false
	}
	b.tokens[provider] = t - 1
	return true
}

// Left is how many calls of provider's budget are left now (refilled
// evenly over the hour); -1 for a provider without a budget.
func (b *Budget) Left(provider string) float64 {
	if b == nil {
		return -1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	rate, ok := b.perHour[provider]
	if !ok {
		return -1
	}
	t, seen := b.tokens[provider]
	if !seen {
		return rate
	}
	return min(rate, t+b.now().Sub(b.updated[provider]).Hours()*rate)
}

// Refused counts calls refused per provider since start.
func (b *Budget) Refused() map[string]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]int64{}
	for k, v := range b.spent {
		out[k] = v
	}
	return out
}

// Fetcher wraps a provider fetcher with the budget (by q.Provider).
type Fetcher struct {
	Fetcher interface {
		Fetch(context.Context, model.Query) (model.Result, error)
	}
	Budget *Budget
}

func (f Fetcher) Fetch(ctx context.Context, q model.Query) (model.Result, error) {
	if !f.Budget.Take(q.Provider) {
		return model.Result{}, &model.FetchError{Code: "upstream_budget", Status: 503, RetryAfter: time.Minute}
	}
	return f.Fetcher.Fetch(ctx, q)
}

// Transport charges requests to the provider owning the host.
type Transport struct {
	Base   http.RoundTripper
	Budget *Budget
}

// Provider maps an upstream host to its budget name ("" when none).
func Provider(host string) string {
	host = strings.ToLower(host)
	switch {
	case host == "api.themoviedb.org":
		return "tmdb"
	case strings.HasSuffix(host, "musicbrainz.org"):
		return "musicbrainz"
	case strings.HasSuffix(host, "listenbrainz.org"):
		return "listenbrainz"
	case strings.HasSuffix(host, "deezer.com"):
		return "deezer"
	case host == "wikidata.org" || strings.HasSuffix(host, ".wikidata.org"):
		return "wikidata"
	case host == "wikipedia.org" || strings.HasSuffix(host, ".wikipedia.org"):
		return "wikipedia"
	case host == "graphql.anilist.co":
		return "anilist"
	case host == "webservice.fanart.tv":
		return "fanart"
	case host == "api.mdblist.com":
		return "mdblist"
	}
	return ""
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if p := Provider(r.URL.Hostname()); p != "" && !t.Budget.Take(p) {
		return nil, ErrSpent
	}
	// Every budgeted call keeps the shared MetaBrainz pace and reports to
	// the connection health watch.
	return upstream.Transport{Next: t.Base}.RoundTrip(r)
}

// Client is an http.Client with the given timeout, charged to the budget.
func Client(b *Budget, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: Transport{Budget: b}}
}
