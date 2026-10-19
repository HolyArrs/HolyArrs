package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// top10TTL is how long a Netflix Top 10 list from the metadata server is
// kept; the server itself changes it once a week.
const top10TTL = time.Hour

// top10Miss is how long a missing list or a failed read is remembered.
const top10Miss = 5 * time.Minute

const top10Max = 500

type top10Entry struct {
	at   time.Time
	list metadata.Top10
	err  error
}

type top10Cache struct {
	mu    sync.Mutex
	lists map[string]top10Entry
}

// top10View is one ranked card of the row.
type top10View struct {
	discoverItemView
	Rank   int    `json:"rank"`
	Season string `json:"season_title,omitempty"`
}

// top10List answers GET /api/v1/top10?kind=movie|show&region=global|<ISO>
// &lang=en|non-en: the metadata server's Netflix Top 10, matched titles
// only (one card per title, best rank), with the Library joined.
func (a *API) top10List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, region, lang := q.Get("kind"), strings.ToUpper(q.Get("region")), q.Get("lang")
	if region == "" || region == "GLOBAL" {
		region = "global"
		if lang == "" {
			lang = "en"
		}
	} else {
		lang = ""
	}
	if (kind != "movie" && kind != "show") || (region == "global" && lang != "en" && lang != "non-en") || (region != "global" && !isCountry(region)) {
		fail(w, http.StatusBadRequest, "kind must be movie or show, region global or a country code, lang en or non-en")
		return
	}
	if a.s.Top10 == nil || len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusNotFound, "no Top 10 list here")
		return
	}
	if !a.discover.allow(a.person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	if !a.knownRegion(region) {
		fail(w, http.StatusNotFound, "no Top 10 list for that region")
		return
	}
	list, err := a.cachedTop10(r.Context(), kind, region, lang)
	if errors.Is(err, metadata.ErrNotFound) {
		fail(w, http.StatusNotFound, "no Top 10 list for that region")
		return
	}
	if err != nil {
		fail(w, http.StatusBadGateway, "the metadata server did not answer; try again shortly")
		return
	}
	// Every rank is kept: a title TMDB does not know is listed by its
	// Netflix name only (not for a Kid, whose limit needs a rating), and a
	// show in the list twice (two seasons) is listed twice.
	var items []metadata.DiscoverItem
	seen := map[string]bool{}
	for _, it := range list.Items {
		if it.TMDBID <= 0 {
			continue
		}
		id := strconv.FormatInt(it.TMDBID, 10)
		if seen[id] {
			continue
		}
		seen[id] = true
		d := metadata.DiscoverItem{TMDBID: id, Kind: kind, Name: it.Title, Genres: []string{}}
		if it.PosterPath != "" {
			d.Poster = metadata.ImageBase + "/w342" + it.PosterPath
		}
		items = append(items, d)
	}
	items, err = a.dropNeverAdd(r.Context(), items)
	if err != nil {
		internal(w, err)
		return
	}
	kid := a.kidMax(r)
	items = a.kidItems(r.Context(), kid, items, 10)
	joined, err := a.joinItems(r.Context(), items, a.person(r).UserID, 10)
	if err != nil {
		internal(w, err)
		return
	}
	byID := map[string]discoverItemView{}
	for _, v := range joined {
		byID[v.TMDBID] = v
	}
	out := make([]top10View, 0, len(list.Items))
	for _, it := range list.Items {
		if it.TMDBID <= 0 {
			if kid == "" {
				out = append(out, top10View{discoverItemView: discoverItemView{DiscoverItem: metadata.DiscoverItem{Kind: kind, Name: it.Title, Genres: []string{}}}, Rank: it.Rank, Season: it.Season})
			}
			continue
		}
		if v, ok := byID[strconv.FormatInt(it.TMDBID, 10)]; ok {
			out = append(out, top10View{discoverItemView: v, Rank: it.Rank, Season: it.Season})
		}
	}
	a.exploreCaching(w, r, "private, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"week": list.Week, "region": region, "regions": list.Regions, "items": out})
}

func isCountry(r string) bool {
	return len(r) == 2 && r[0] >= 'A' && r[0] <= 'Z' && r[1] >= 'A' && r[1] <= 'Z'
}

// knownRegion: once a list with the server's regions is cached, only those
// regions are asked for.
func (a *API) knownRegion(region string) bool {
	if region == "global" {
		return true
	}
	c := &a.top10
	c.mu.Lock()
	defer c.mu.Unlock()
	var newest top10Entry
	for _, e := range c.lists {
		if e.err == nil && len(e.list.Regions) > 0 && e.at.After(newest.at) {
			newest = e
		}
	}
	if newest.at.IsZero() {
		return true
	}
	for _, r := range newest.list.Regions {
		if r == region {
			return true
		}
	}
	return false
}

// cachedTop10 keeps lists an hour, and missing lists or failures five
// minutes; a failure is answered from an older good copy when there is one.
// A full cache drops its oldest entry.
func (a *API) cachedTop10(ctx context.Context, kind, region, lang string) (metadata.Top10, error) {
	key := kind + "|" + region + "|" + lang
	c := &a.top10
	c.mu.Lock()
	e, ok := c.lists[key]
	c.mu.Unlock()
	if ok && ((e.err == nil && time.Since(e.at) < top10TTL) || (e.err != nil && time.Since(e.at) < top10Miss)) {
		return e.list, e.err
	}
	server := kind
	if kind == "show" {
		server = "tv"
	}
	l, err := a.s.Top10(ctx, server, region, lang)
	if err != nil && ok && e.err == nil && !errors.Is(err, metadata.ErrNotFound) {
		return e.list, nil
	}
	if err != nil && ctx.Err() != nil {
		return l, err
	}
	c.mu.Lock()
	if c.lists == nil {
		c.lists = map[string]top10Entry{}
	}
	if _, has := c.lists[key]; !has && len(c.lists) >= top10Max {
		oldest := ""
		for k, v := range c.lists {
			if oldest == "" || v.at.Before(c.lists[oldest].at) {
				oldest = k
			}
		}
		delete(c.lists, oldest)
	}
	c.lists[key] = top10Entry{at: time.Now(), list: l, err: err}
	c.mu.Unlock()
	return l, err
}
