package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// rankedView is one ranked card of a top row.
type rankedView struct {
	discoverItemView
	Rank   int    `json:"rank"`
	Season string `json:"season_title,omitempty"`
}

func isCountry(r string) bool {
	return len(r) == 2 && r[0] >= 'A' && r[0] <= 'Z' && r[1] >= 'A' && r[1] <= 'Z'
}

// serviceTopTTL is how long the metadata server's per-service lists are
// kept; the server refreshes them every 12 hours.
const serviceTopTTL = 12 * time.Hour

// serviceTopMiss is how long a failed read is remembered.
const serviceTopMiss = 5 * time.Minute

// serviceTopProviders maps the server's service names to TMDB watch
// provider ids (the ids the picker uses).
var serviceTopProviders = map[string]int{"netflix": 8, "prime": 9, "disney": 337, "apple": 350, "hbo": 1899, "hulu": 15, "paramount": 531, "crunchyroll": 283, "peacock": 386, "starz": 43, "tubi": 73}

type serviceTopEntry struct {
	at   time.Time
	list metadata.ServiceTop
	err  error
}

type serviceTopCache struct {
	mu      sync.Mutex
	regions map[string]serviceTopEntry
}

// cachedServiceTop keeps a region's lists 12 hours and a failure five
// minutes; a failure is answered from an older good copy when there is one.
func (a *API) cachedServiceTop(ctx context.Context, region string) (metadata.ServiceTop, error) {
	c := &a.serviceTop
	c.mu.Lock()
	e, ok := c.regions[region]
	c.mu.Unlock()
	if ok && ((e.err == nil && time.Since(e.at) < serviceTopTTL) || (e.err != nil && time.Since(e.at) < serviceTopMiss)) {
		return e.list, e.err
	}
	l, err := a.s.ServiceTop(ctx, region)
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

// warmServiceTop reads Trending on streaming for every region a member
// has, so the first Explore head after a restart can keep its rows
// apart from that row's titles without waiting on it.
func (a *API) warmServiceTop(ctx context.Context) {
	if a.s.Store == nil || a.s.ServiceTop == nil {
		return
	}
	regions := map[string]bool{}
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT DISTINCT region FROM explore_prefs WHERE region != ''")
	if err != nil {
		return
	}
	for rows.Next() {
		var r string
		if rows.Scan(&r) == nil && isCountry(r) {
			regions[r] = true
		}
	}
	rows.Close()
	for r := range regions {
		if ctx.Err() != nil {
			return
		}
		_, _ = a.cachedServiceTop(ctx, r)
	}
}

// keptServiceTop is every service's titles for region and the switch's
// kinds, from the kept lists only (no outside call); nil when none is kept.
func (a *API) keptServiceTop(region, sw string) []engine.Item {
	c := &a.serviceTop
	c.mu.Lock()
	e, ok := c.regions[region]
	c.mu.Unlock()
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
			if kind := serviceKind(it.Kind); i < topServicesMax && it.TMDBID > 0 && kinds[kind] {
				out = append(out, engine.Item{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind})
			}
		}
	}
	return out
}

// serviceTopView is one service's ranked row.
type serviceTopView struct {
	// Service is the TMDB watch provider id; Key the server's name.
	Service   int    `json:"service"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
	// AgeSeconds is the list's age; Stale is true after 14 days.
	AgeSeconds int64 `json:"age_seconds"`
	Stale      bool  `json:"stale"`
	// Ranked is true for the service's own top list; TMDB popularity is
	// not a ranking.
	Ranked bool         `json:"ranked"`
	Items  []rankedView `json:"items"`
}

func serviceKind(k string) string {
	if k == "tv" {
		return "show"
	}
	return k
}

// exploreServiceTop answers GET /api/v1/explore/service-top?type=all|movies
// |shows&region=<ISO>: every streaming service's top list in one answer,
// so switching service is instant on the page. Each list keeps the
// service's order, split by the type's kinds, at most 10 titles; Library,
// Kid limits and never-add apply as on the other rows.
func (a *API) exploreServiceTop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := exploreSwitch(q.Get("type"))
	region := strings.ToUpper(q.Get("region"))
	if !ok || !isCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows and region a country code")
		return
	}
	out := map[string]any{"region": region, "attribution": "", "services": []serviceTopView{}}
	if a.s.ServiceTop == nil {
		a.exploreCaching(w, r, "private, max-age=300")
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	kinds := map[string]bool{}
	for _, k := range engine.Kinds(sw) {
		if len(a.kindsOn(k)) > 0 {
			kinds[k] = true
		}
	}
	list, err := a.cachedServiceTop(r.Context(), region)
	if errors.Is(err, metadata.ErrNotFound) || (err == nil && len(kinds) == 0) {
		a.exploreCaching(w, r, "private, max-age=300")
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, "the metadata server did not answer; try again shortly")
		return
	}
	// Every title once, joined in one pass, then handed back per service.
	var all []metadata.DiscoverItem
	seen := map[string]bool{}
	for _, sv := range list.Services {
		for _, it := range sv.Items {
			kind := serviceKind(it.Kind)
			key := kind + strconv.FormatInt(it.TMDBID, 10)
			if it.TMDBID <= 0 || !kinds[kind] || seen[key] {
				continue
			}
			seen[key] = true
			d := metadata.DiscoverItem{TMDBID: strconv.FormatInt(it.TMDBID, 10), Kind: kind, Name: it.Title, Genres: []string{}}
			if it.PosterPath != "" {
				d.Poster = metadata.ImageBase + "/w342" + it.PosterPath
			}
			all = append(all, d)
		}
	}
	all, err = a.dropNeverAdd(r.Context(), all)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	all = a.kidItems(r.Context(), a.kidMax(r), all, len(all))
	joined, err := a.joinItems(r.Context(), all, a.person(r).UserID, len(all))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	a.fillTrailerKeys(r, joined)
	a.fillCardArt(r, joined)
	byKey := map[string]discoverItemView{}
	for _, v := range joined {
		byKey[v.Kind+v.TMDBID] = v
	}
	views := []serviceTopView{}
	for _, sv := range list.Services {
		v := serviceTopView{Service: serviceTopProviders[sv.Service], Key: sv.Service, Name: sv.Name, Source: sv.Source, FetchedAt: sv.FetchedAt, AgeSeconds: sv.AgeSeconds, Stale: sv.Stale, Ranked: sv.Source == "streaming-availability", Items: []rankedView{}}
		for _, it := range sv.Items {
			if j, ok := byKey[serviceKind(it.Kind)+strconv.FormatInt(it.TMDBID, 10)]; ok && len(v.Items) < topServicesMax {
				v.Items = append(v.Items, rankedView{discoverItemView: j, Rank: len(v.Items) + 1})
			}
		}
		if v.Service != 0 && len(v.Items) > 0 {
			views = append(views, v)
		}
	}
	out["attribution"], out["services"] = list.Attribution, views
	a.exploreCaching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, out)
}

type streamingMark = discover.StreamingMark

// streamingMarks maps kind:tmdb to the first of the member's services
// (in their order) whose kept list has it. It never fetches.
func (a *API) streamingMarks(region string, services []int) map[string]streamingMark {
	if region == "" || len(services) == 0 {
		return nil
	}
	c := &a.serviceTop
	c.mu.Lock()
	e, ok := c.regions[region]
	c.mu.Unlock()
	if !ok || e.err != nil {
		return nil
	}
	out := map[string]streamingMark{}
	for _, id := range services {
		for _, sv := range e.list.Services {
			if serviceTopProviders[sv.Service] != id {
				continue
			}
			for i, it := range sv.Items {
				if it.TMDBID <= 0 {
					continue
				}
				key := serviceKind(it.Kind) + ":" + strconv.FormatInt(it.TMDBID, 10)
				if _, seen := out[key]; seen {
					continue
				}
				m := streamingMark{Service: id, Name: sv.Name}
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

// fillStreaming marks the cards whose title is on one of the member's
// services (explore_prefs), from memory: one small read; titles whose
// watch providers are not kept yet are read in the background.
func (a *API) fillStreaming(r *http.Request, rows []exploreRowView) {
	if a.s.Store == nil {
		return
	}
	var services, region string
	if err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT services, region FROM explore_prefs WHERE member_id = ?", a.person(r).UserID).Scan(&services, &region); err != nil {
		return
	}
	var ids []int
	for _, s := range strings.Split(services, "|") {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	region = strings.ToUpper(region)
	var keys []string
	for i := range rows {
		for _, it := range rows[i].Items {
			keys = append(keys, it.Kind+":"+it.TMDBID)
		}
	}
	// TMDB watch providers first; a service's own top list adds its rank
	// (and covers titles not read yet).
	watch := a.watchOn.Marks(region, ids, keys)
	top := a.streamingMarks(region, ids)
	if len(watch) == 0 && len(top) == 0 {
		return
	}
	for i := range rows {
		for j := range rows[i].Items {
			it := &rows[i].Items[j]
			k := it.Kind + ":" + it.TMDBID
			m, ok := watch[k]
			if t, onTop := top[k]; onTop && (!ok || t.Service == m.Service) {
				m, ok = t, true
			}
			if ok {
				it.Streaming = &m
			}
		}
	}
}
