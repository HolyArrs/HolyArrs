package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/ratings"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// discoverTitles are the row headings the app may show.
var discoverTitles = map[string]string{
	"trending": "Trending this week", "popular": "Popular", "upcoming": "Coming soon",
	"similar": "Because you have it", "genre": "More in this genre",
}

// discoverTTL is how long a row from TMDB is kept.
var discoverTTL = map[string]time.Duration{
	"trending": 6 * time.Hour, "popular": 12 * time.Hour, "upcoming": 12 * time.Hour,
	"similar": 24 * time.Hour, "genre": 24 * time.Hour, "details": 24 * time.Hour,
}

const (
	discoverMax       = 20
	discoverPerMinute = 30
	maxDiscoverCache  = 500
)

type discoverEntry struct {
	at      time.Time
	items   []metadata.DiscoverItem
	details *metadata.Details
}

// discoverState holds the row cache and each person's request times.
type discoverState struct {
	mu    sync.Mutex
	cache map[string]discoverEntry
	hits  map[int64][]time.Time
}

// allow counts one request for a person; false when over the minute limit.
func (d *discoverState) allow(who int64, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hits == nil {
		d.hits = map[int64][]time.Time{}
	}
	kept := d.hits[who][:0]
	for _, t := range d.hits[who] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= discoverPerMinute {
		d.hits[who] = kept
		return false
	}
	d.hits[who] = append(kept, now)
	return true
}

func (d *discoverState) get(key string) (discoverEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.cache[key]
	return e, ok
}

func (d *discoverState) put(key string, e discoverEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cache == nil || len(d.cache) >= maxDiscoverCache {
		d.cache = map[string]discoverEntry{}
	}
	d.cache[key] = e
}

// discoverItemView is one discover item with what the Library knows of it.
type discoverItemView struct {
	metadata.DiscoverItem
	InLibrary bool      `json:"in_library"`
	TitleID   int64     `json:"title_id,omitempty"`
	Ask       *askBrief `json:"ask"`
}

// kindsOn are the asked kinds whose part is on.
func (a *API) kindsOn(kind string) []string {
	var out []string
	for _, k := range []string{"movie", "show"} {
		if kind != "" && kind != k {
			continue
		}
		if p, _ := parts.ForTitleKind(k); a.partIsOn(p) {
			out = append(out, k)
		}
	}
	return out
}

// discoverFail answers a provider error: 501 not set up, 503 rate limited,
// 404 unknown, 502 anything else.
func discoverFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, metadata.ErrNoAccess):
		fail(w, http.StatusNotImplemented, "TMDB is not set up; add a TMDB read access token in Settings")
	case errors.Is(err, metadata.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusServiceUnavailable, "TMDB is busy; try again in a minute")
	case errors.Is(err, metadata.ErrNotFound):
		fail(w, http.StatusNotFound, "TMDB does not know that title")
	default:
		fail(w, http.StatusBadGateway, "TMDB did not answer; try again shortly")
	}
}

// cachedRow reads one row of one kind, from the cache while fresh. A
// rate-limited TMDB is answered from a stale copy when there is one.
func (a *API) cachedRow(ctx context.Context, d metadata.Discoverer, row, kind, ref, genre string) ([]metadata.DiscoverItem, error) {
	key := row + "|" + kind + "|" + ref + "|" + genre
	e, ok := a.discover.get(key)
	if ok && time.Since(e.at) < discoverTTL[row] {
		return e.items, nil
	}
	items, err := d.Discover(ctx, row, kind, ref, genre)
	if err != nil {
		if ok && errors.Is(err, metadata.ErrRateLimited) {
			return e.items, nil
		}
		return nil, err
	}
	a.discover.put(key, discoverEntry{at: time.Now(), items: items})
	return items, nil
}

// libraryJoin finds the Library title, the Never-add entry and the open ask
// for one item.
func (a *API) libraryJoin(ctx context.Context, it metadata.DiscoverItem, me int64) (discoverItemView, bool, error) {
	v := discoverItemView{DiscoverItem: it}
	if v.Genres == nil {
		v.Genres = []string{}
	}
	db := a.s.Store.Read()
	err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?",
		it.Kind, it.TMDBID).Scan(&v.TitleID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	v.InLibrary = v.TitleID != 0
	var never int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM never_add WHERE kind = ? AND json_extract(source_ids, '$.tmdb') = ?",
		it.Kind, it.TMDBID).Scan(&never); err != nil {
		return v, false, err
	}
	if it.Kind == "show" {
		v.Ask, err = a.openShowAsk(ctx, map[string]string{"tmdb": it.TMDBID}, me)
	} else {
		v.Ask, err = a.openAsk(ctx, it.TMDBID, me)
	}
	return v, never > 0, err
}

func (a *API) joinItems(ctx context.Context, items []metadata.DiscoverItem, me int64, n int) ([]discoverItemView, error) {
	out := []discoverItemView{}
	for _, it := range items {
		if len(out) == n {
			break
		}
		v, never, err := a.libraryJoin(ctx, it, me)
		if err != nil {
			return nil, err
		}
		if !never {
			out = append(out, v)
		}
	}
	return out, nil
}

func (a *API) discoverRow(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	row, kind, idText, genre := q.Get("row"), q.Get("kind"), q.Get("id"), q.Get("g")
	if _, ok := discoverTitles[row]; !ok {
		fail(w, http.StatusBadRequest, "row must be trending, popular, upcoming, similar or genre")
		return
	}
	if kind != "" && kind != "movie" && kind != "show" {
		fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	var titleID int64
	if row == "similar" {
		n, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || n <= 0 {
			fail(w, http.StatusBadRequest, "similar needs id=<Library title id>")
			return
		}
		titleID = n
	}
	if row == "genre" && (genre == "" || len(genre) > 100) {
		fail(w, http.StatusBadRequest, "genre needs g=<genre name>")
		return
	}
	me := a.person(r).UserID
	if !a.discover.allow(me, time.Now()) {
		w.Header().Set("Retry-After", "60")
		fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	if !ok {
		fail(w, http.StatusNotImplemented, "TMDB is not set up; add a TMDB read access token in Settings")
		return
	}
	kinds := a.kindsOn(kind)
	ref := ""
	if row == "similar" {
		var t domain.Title
		if err := store.Get(r.Context(), a.s.Store.Read(), "titles", titleID, &t); err != nil || t.State == "removed" || !a.titleAllowed(r, t) {
			fail(w, http.StatusNotFound, "no such Title")
			return
		}
		ref = t.SourceIDs["tmdb"]
		kinds = a.kindsOn(t.Kind)
		if (t.Kind != "movie" && t.Kind != "show") || (kind != "" && kind != t.Kind) || ref == "" {
			kinds = nil
		}
	}
	var lists [][]metadata.DiscoverItem
	for _, k := range kinds {
		items, err := a.cachedRow(r.Context(), d, row, k, ref, genre)
		if err != nil {
			discoverFail(w, err)
			return
		}
		lists = append(lists, items)
	}
	// Both kinds: take turns, so neither crowds the other out.
	var merged []metadata.DiscoverItem
	for i := 0; ; i++ {
		added := false
		for _, l := range lists {
			if i < len(l) {
				merged = append(merged, l[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	// A Kid gets only the items their limit allows; the ratings come from
	// the Library, the details cache or TMDB, and unknown ones stay hidden.
	merged = a.kidItems(r.Context(), a.kidMax(r), merged, discoverMax)
	out, err := a.joinItems(r.Context(), merged, me, discoverMax)
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"row": row, "title": discoverTitles[row], "items": out})
}

// metadataDetails answers a movie or show page from TMDB, in the Library
// or not, with the Library and Wishlist joined.
func (a *API) metadataDetails(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusConflict, "that part is off")
		return
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	if !ok {
		fail(w, http.StatusNotImplemented, "TMDB is not set up; add a TMDB read access token in Settings")
		return
	}
	key := "details|" + kind + "|" + id
	e, cached := a.discover.get(key)
	if !cached || time.Since(e.at) >= discoverTTL["details"] {
		det, err := d.Details(r.Context(), kind, id)
		switch {
		case err == nil:
			e = discoverEntry{at: time.Now(), details: &det}
			a.discover.put(key, e)
		case !(cached && errors.Is(err, metadata.ErrRateLimited)):
			discoverFail(w, err)
			return
		}
	}
	max := a.kidMax(r)
	if max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = e.details.Rating
		}
		if !ratings.Allows(max, kind, rating) {
			fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	me := a.person(r).UserID
	head, never, err := a.libraryJoin(r.Context(), e.details.DiscoverItem, me)
	if err != nil {
		internal(w, err)
		return
	}
	similar, err := a.joinItems(r.Context(), a.kidItems(r.Context(), max, e.details.Similar, 0), me, len(e.details.Similar))
	if err != nil {
		internal(w, err)
		return
	}
	det := *e.details
	if det.People == nil {
		det.People = []metadata.DetailPerson{}
	}
	writeJSON(w, http.StatusOK, struct {
		metadata.Details
		Similar   []discoverItemView `json:"similar"`
		InLibrary bool               `json:"in_library"`
		TitleID   int64              `json:"title_id,omitempty"`
		Ask       *askBrief          `json:"ask"`
		NeverAdd  bool               `json:"never_add"`
	}{Details: det, Similar: similar, InLibrary: head.InLibrary, TitleID: head.TitleID, Ask: head.Ask, NeverAdd: never})
}
