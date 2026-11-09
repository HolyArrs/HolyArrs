package discover

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// IsCountry reports whether r is a two-letter upper-case country code.
func IsCountry(r string) bool {
	return len(r) == 2 && r[0] >= 'A' && r[0] <= 'Z' && r[1] >= 'A' && r[1] <= 'Z'
}

// serviceTopTTL is how long the metadata server's per-service lists are
// kept; the server refreshes them every 12 hours.
const serviceTopTTL = 12 * time.Hour

// serviceTopMiss is how long a failed read is remembered.
const serviceTopMiss = 5 * time.Minute

// ServiceTopProviders maps the server's service names to TMDB watch
// provider ids (the ids the picker uses).
var ServiceTopProviders = map[string]int{"netflix": 8, "prime": 9, "disney": 337, "apple": 350, "hbo": 1899, "hulu": 15, "paramount": 531, "crunchyroll": 283, "peacock": 386, "starz": 43, "tubi": 73}

// ServiceKind turns the server's "tv" into our "show".
func ServiceKind(k string) string {
	if k == "tv" {
		return "show"
	}
	return k
}

type serviceTopEntry struct {
	at   time.Time
	list metadata.ServiceTop
	err  error
}

// ServiceTopCache keeps each region's per-service top lists.
type ServiceTopCache struct {
	mu      sync.Mutex
	regions map[string]serviceTopEntry
}

// Put keeps l as region's lists, read now.
func (c *ServiceTopCache) Put(region string, l metadata.ServiceTop) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.regions == nil {
		c.regions = map[string]serviceTopEntry{}
	}
	c.regions[region] = serviceTopEntry{at: time.Now(), list: l}
}

func (c *ServiceTopCache) entry(region string) (serviceTopEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.regions[region]
	return e, ok
}

// Get keeps a region's lists 12 hours and a failure five minutes; a
// failure is answered from an older good copy when there is one.
func (c *ServiceTopCache) Get(ctx context.Context, region string, fetch func(context.Context, string) (metadata.ServiceTop, error)) (metadata.ServiceTop, error) {
	e, ok := c.entry(region)
	if ok && ((e.err == nil && time.Since(e.at) < serviceTopTTL) || (e.err != nil && time.Since(e.at) < serviceTopMiss)) {
		return e.list, e.err
	}
	l, err := fetch(ctx, region)
	if err != nil && ok && e.err == nil {
		return e.list, nil
	}
	if err != nil && ctx.Err() != nil {
		return l, err
	}
	c.mu.Lock()
	if c.regions == nil {
		c.regions = map[string]serviceTopEntry{}
	}
	if _, has := c.regions[region]; has || len(c.regions) < 300 {
		c.regions[region] = serviceTopEntry{at: time.Now(), list: l, err: err}
	}
	c.mu.Unlock()
	return l, err
}

// Kept is every service's first max titles for region and the switch's
// kinds, from the kept lists only (no outside call); nil when none is kept.
func (c *ServiceTopCache) Kept(region, sw string, max int) []engine.Item {
	e, ok := c.entry(region)
	if !ok || e.err != nil {
		return nil
	}
	kinds := map[string]bool{}
	for _, k := range engine.Kinds(sw) {
		kinds[k] = true
	}
	var out []engine.Item
	for _, sv := range e.list.Services {
		for i, it := range sv.Items {
			if kind := ServiceKind(it.Kind); i < max && it.TMDBID > 0 && kinds[kind] {
				out = append(out, engine.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind})
			}
		}
	}
	return out
}

// Marks maps kind:tmdb to the first of the member's services (in their
// order) whose kept list has it. It never fetches.
func (c *ServiceTopCache) Marks(region string, services []int) map[string]StreamingMark {
	if region == "" || len(services) == 0 {
		return nil
	}
	e, ok := c.entry(region)
	if !ok || e.err != nil {
		return nil
	}
	out := map[string]StreamingMark{}
	for _, id := range services {
		for _, sv := range e.list.Services {
			if ServiceTopProviders[sv.Service] != id {
				continue
			}
			for i, it := range sv.Items {
				if it.TMDBID <= 0 {
					continue
				}
				key := ServiceKind(it.Kind) + ":" + strconv.FormatInt(it.TMDBID, 10)
				if _, seen := out[key]; seen {
					continue
				}
				m := StreamingMark{Service: id, Name: sv.Name}
				if sv.Source == "streaming-availability" {
					m.Rank = it.Rank
					if m.Rank <= 0 {
						m.Rank = i + 1
					}
				}
				out[key] = m
			}
		}
	}
	return out
}
