package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
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
	// refresh marks lists being read again in the background.
	refresh sync.Map
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
	status, msg, out := a.top10Answer(r.Context(), r, kind, region, lang)
	if status != http.StatusOK {
		if status == http.StatusInternalServerError {
			internal(w, errors.New(msg))
			return
		}
		fail(w, status, msg)
		return
	}
	a.exploreCaching(w, r, "private, max-age=300")
	writeJSON(w, http.StatusOK, out)
}

// top10Answer is the Top 10 answer for one list: the status, the failure
// words, or the body (week, region, regions, items).
func (a *API) top10Answer(ctx context.Context, r *http.Request, kind, region, lang string) (int, string, map[string]any) {
	if !a.knownRegion(region) {
		return http.StatusNotFound, "no Top 10 list for that region", nil
	}
	list, err := a.cachedTop10(ctx, kind, region, lang)
	if errors.Is(err, metadata.ErrNotFound) {
		return http.StatusNotFound, "no Top 10 list for that region", nil
	}
	if err != nil {
		return http.StatusBadGateway, "the metadata server did not answer; try again shortly", nil
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
	items, err = a.dropNeverAdd(ctx, items)
	if err != nil {
		return http.StatusInternalServerError, err.Error(), nil
	}
	kid := a.kidMax(r)
	items = a.kidItems(ctx, kid, items, 10)
	joined, err := a.joinItems(withFreshRow(ctx, "top10"), items, a.person(r).UserID, 10)
	if err != nil {
		return http.StatusInternalServerError, err.Error(), nil
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
	return http.StatusOK, "", map[string]any{"week": list.Week, "region": region, "regions": list.Regions, "items": out}
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
	if !ok {
		// After a restart the list comes from disk at once; an old copy is
		// answered now and read again in the background.
		if d, found := a.diskTop10(ctx, key); found {
			c.mu.Lock()
			if c.lists == nil {
				c.lists = map[string]top10Entry{}
			}
			c.lists[key] = d
			c.mu.Unlock()
			if time.Since(d.at) >= top10TTL {
				go a.refreshTop10(kind, region, lang)
			}
			return d.list, nil
		}
	}
	return a.fetchTop10(ctx, kind, region, lang, e, ok)
}

// top10Keep is how long a list stays on disk to be answered after a restart.
const top10Keep = 14 * 24 * time.Hour

type diskTop10Body struct {
	At   time.Time      `json:"at"`
	List metadata.Top10 `json:"list"`
}

func (a *API) diskTop10(ctx context.Context, key string) (top10Entry, bool) {
	if a.s.Store == nil {
		return top10Entry{}, false
	}
	b, found, err := store.LookupCache{Store: a.s.Store}.Get(ctx, "top10|"+key, time.Now())
	if err != nil || !found {
		return top10Entry{}, false
	}
	var d diskTop10Body
	if json.Unmarshal(b, &d) != nil {
		return top10Entry{}, false
	}
	return top10Entry{at: d.At, list: d.List}, true
}

// refreshTop10 reads one list again in the background (one at a time per list).
func (a *API) refreshTop10(kind, region, lang string) {
	key := kind + "|" + region + "|" + lang
	if _, busy := a.top10.refresh.LoadOrStore(key, true); busy {
		return
	}
	defer a.top10.refresh.Delete(key)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := &a.top10
	c.mu.Lock()
	e, ok := c.lists[key]
	c.mu.Unlock()
	_, _ = a.fetchTop10(ctx, kind, region, lang, e, ok)
}

func (a *API) fetchTop10(ctx context.Context, kind, region, lang string, e top10Entry, ok bool) (metadata.Top10, error) {
	key := kind + "|" + region + "|" + lang
	c := &a.top10
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
	now := time.Now()
	c.lists[key] = top10Entry{at: now, list: l, err: err}
	c.mu.Unlock()
	if err == nil && a.s.Store != nil {
		if b, jerr := json.Marshal(diskTop10Body{At: now, List: l}); jerr == nil {
			_ = store.LookupCache{Store: a.s.Store}.Put(context.WithoutCancel(ctx), "top10|"+key, b, now, now.Add(top10Keep))
		}
	}
	return l, err
}

// warmTop10 loads the Top 10 lists kept on disk into memory at start, so
// the first Explore head after a restart makes no outside call; lists past
// their hour are read again in the background.
func (a *API) warmTop10(ctx context.Context) {
	if a.s.Store == nil || a.s.Top10 == nil {
		return
	}
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT key FROM lookup_cache WHERE key LIKE 'top10|%' AND expires_at > ?", store.FormatTime(time.Now()))
	if err != nil {
		return
	}
	var keys []string
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			keys = append(keys, strings.TrimPrefix(k, "top10|"))
		}
	}
	rows.Close()
	for _, key := range keys {
		parts := strings.Split(key, "|")
		if len(parts) != 3 {
			continue
		}
		_, _ = a.cachedTop10(ctx, parts[0], parts[1], parts[2])
	}
}
