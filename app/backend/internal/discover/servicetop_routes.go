package discover

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// TopServicesMax is how many ranked titles a top row shows.
const TopServicesMax = 10

// RankedView is one ranked card of a top row.
type RankedView struct {
	ItemView
	Rank   int    `json:"rank"`
	Season string `json:"season_title,omitempty"`
}

func (h *Handlers) serviceTopFetch() ServiceTopFetch {
	if h.ServiceTop == nil {
		return nil
	}
	return h.ServiceTop()
}

func (h *Handlers) cachedServiceTop(ctx context.Context, region string) (metadata.ServiceTop, error) {
	return h.ServiceTopCache.Get(ctx, region, h.serviceTopFetch())
}

// WarmServiceTop reads Trending on streaming for every region a member
// has, so the first Explore head after a restart can keep its rows
// apart from that row's titles without waiting on it.
func (h *Handlers) WarmServiceTop(ctx context.Context) {
	if h.Store == nil || h.serviceTopFetch() == nil {
		return
	}
	regions := map[string]bool{}
	rows, err := h.Store.Read().QueryContext(ctx, "SELECT DISTINCT region FROM explore_prefs WHERE region != ''")
	if err != nil {
		return
	}
	for rows.Next() {
		var r string
		if rows.Scan(&r) == nil && IsCountry(r) {
			regions[r] = true
		}
	}
	rows.Close()
	for r := range regions {
		if ctx.Err() != nil {
			return
		}
		_, _ = h.cachedServiceTop(ctx, r)
	}
}

// ServiceTopView is one service's ranked row.
type ServiceTopView struct {
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
	Items  []RankedView `json:"items"`
}

// exploreServiceTop answers GET /api/v1/explore/service-top?type=all|movies
// |shows&region=<ISO>: every streaming service's top list in one answer,
// so switching service is instant on the page. Each list keeps the
// service's order, split by the type's kinds, at most 10 titles; Library,
// Kid limits and never-add apply as on the other rows.
func (h *Handlers) exploreServiceTop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := ExploreSwitch(q.Get("type"))
	region := strings.ToUpper(q.Get("region"))
	if !ok || !IsCountry(region) {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows and region a country code")
		return
	}
	out := map[string]any{"region": region, "attribution": "", "services": []ServiceTopView{}}
	if h.serviceTopFetch() == nil {
		h.Caching(w, r, "private, max-age=300")
		webserver.WriteJSON(w, http.StatusOK, out)
		return
	}
	if !h.Cache.Allow(h.UserID(r), time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	kinds := map[string]bool{}
	for _, k := range engine.Kinds(sw) {
		if len(h.KindsOn(k)) > 0 {
			kinds[k] = true
		}
	}
	list, err := h.cachedServiceTop(r.Context(), region)
	if errors.Is(err, metadata.ErrNotFound) || (err == nil && len(kinds) == 0) {
		h.Caching(w, r, "private, max-age=300")
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
			kind := ServiceKind(it.Kind)
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
	all, err = h.DropNeverAdd(r.Context(), all)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	all = h.KidItems(r.Context(), h.KidMax(r), all, len(all))
	joined, err := h.JoinItems(r.Context(), all, h.UserID(r), len(all))
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	h.FillTrailerKeys(r, joined)
	h.FillCardArt(r, joined)
	byKey := map[string]ItemView{}
	for _, v := range joined {
		byKey[v.Kind+v.TMDBID] = v
	}
	views := []ServiceTopView{}
	for _, sv := range list.Services {
		v := ServiceTopView{Service: ServiceTopProviders[sv.Service], Key: sv.Service, Name: sv.Name, Source: sv.Source, FetchedAt: sv.FetchedAt, AgeSeconds: sv.AgeSeconds, Stale: sv.Stale, Ranked: sv.Source == "streaming-availability", Items: []RankedView{}}
		for _, it := range sv.Items {
			if j, ok := byKey[ServiceKind(it.Kind)+strconv.FormatInt(it.TMDBID, 10)]; ok && len(v.Items) < TopServicesMax {
				v.Items = append(v.Items, RankedView{ItemView: j, Rank: len(v.Items) + 1})
			}
		}
		if v.Service != 0 && len(v.Items) > 0 {
			views = append(views, v)
		}
	}
	out["attribution"], out["services"] = list.Attribution, views
	h.Caching(w, r, "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, out)
}

// FillStreaming marks the cards whose title is on one of the member's
// services (explore_prefs), from memory: one small read; titles whose
// watch providers are not kept yet are read in the background.
func (h *Handlers) FillStreaming(r *http.Request, rows ...[]ItemView) {
	if h.Store == nil {
		return
	}
	var services, region string
	if err := h.Store.Read().QueryRowContext(r.Context(), "SELECT services, region FROM explore_prefs WHERE member_id = ?", h.UserID(r)).Scan(&services, &region); err != nil {
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
		for _, it := range rows[i] {
			keys = append(keys, it.Kind+":"+it.TMDBID)
		}
	}
	// TMDB watch providers first; a service's own top list adds its rank
	// (and covers titles not read yet).
	watch := h.WatchOn.Marks(region, ids, keys)
	top := h.ServiceTopCache.Marks(region, ids)
	if len(watch) == 0 && len(top) == 0 {
		return
	}
	for i := range rows {
		for j := range rows[i] {
			it := &rows[i][j]
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
