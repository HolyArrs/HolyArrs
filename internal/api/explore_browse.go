package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/explore"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// browseTTL is how long a member's built Browse sections are kept (the
// lists under them are cached by their own TTL, 12 h for discover).
const browseTTL = 6 * time.Hour

// browseCategoryMax is how many titles a category view shows.
const browseCategoryMax = 40

// browseBuild is one member's Browse for one tab, built section by
// section in order so no backdrop repeats across sections.
type browseBuild struct {
	mu       sync.Mutex
	at       time.Time
	used     map[string]bool
	names    map[string]bool
	next     int
	sections map[string][]explore.CategoryTile
}

var browseBuilds sync.Map

// browseStart resets b from the member's page: the page's genre tiles take
// their backdrops and names first.
func (b *browseBuild) start(page []explore.BrowseTile) {
	b.at, b.next = time.Now(), 0
	b.used, b.names, b.sections = map[string]bool{}, map[string]bool{}, map[string][]explore.CategoryTile{}
	for _, t := range page {
		b.used[t.Backdrop] = true
		b.names[t.Name] = true
	}
}

// browsePage is the member's filtered page and its fast genre tiles.
func (a *API) browsePage(ctx context.Context, r *http.Request, sw, topKind, topRegion string) (explore.Page, []explore.BrowseTile, bool, error) {
	pg, _, _, err := a.explorePageFor(ctx, r, a.person(r).UserID, sw, topKind, topRegion)
	if err != nil {
		return pg, nil, false, err
	}
	if pg, err = a.genreRowPage(ctx, r, pg); err != nil {
		return pg, nil, false, err
	}
	kid := pg.Kid || a.kidMax(r) != ""
	return pg, explore.PageGenreTiles(pg, sw, kid, map[string]bool{}), kid, nil
}

// browseItems fetches category c for the tab (both kinds merged by
// popularity), without titles this person may not see.
func (a *API) browseItems(ctx context.Context, r *http.Request, src exploreSource, c explore.Category, sw string, kid bool) ([]explore.Item, error) {
	var lists [][]explore.Item
	for _, q := range c.Requests(sw, kid) {
		items, err := src.List(ctx, q)
		if err != nil {
			return nil, err
		}
		lists = append(lists, items)
	}
	merged := explore.MergeTop(lists, 80)
	refs := make([]metadata.DiscoverItem, len(merged))
	for i, it := range merged {
		refs[i] = metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind}
	}
	if max := a.kidMax(r); max != "" {
		refs = a.kidItems(ctx, max, refs, 0)
	}
	refs, err := a.dropNeverAdd(ctx, refs)
	if err != nil {
		return nil, err
	}
	ok := map[string]bool{}
	for _, it := range refs {
		ok[it.Kind+":"+it.TMDBID] = true
	}
	out := []explore.Item{}
	for _, it := range merged {
		if ok[it.Key()] {
			out = append(out, it)
		}
	}
	return out, nil
}

// browseSectionTiles builds the sections up to id (in order) and returns id's tiles.
func (a *API) browseSectionTiles(ctx context.Context, r *http.Request, sw string, kid bool, pg explore.Page, page []explore.BrowseTile, id string) ([]explore.CategoryTile, error) {
	key := fmt.Sprintf("%p|%d|%s|%t|%s", a, a.person(r).UserID, sw, kid, a.kidMax(r))
	v, _ := browseBuilds.LoadOrStore(key, &browseBuild{})
	b := v.(*browseBuild)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used == nil || time.Since(b.at) >= browseTTL {
		b.start(page)
	}
	if got, ok := b.sections[id]; ok {
		return got, nil
	}
	src := a.liveSource(ctx)
	now := time.Now()
	for b.next < len(explore.BrowseSections) {
		s := explore.BrowseSections[b.next]
		cats := explore.SectionCategories(s.ID, sw, kid, now)
		res := make([]explore.CategoryResult, len(cats))
		errs := make([]error, len(cats))
		var wg sync.WaitGroup
		for i, c := range cats {
			wg.Add(1)
			go func(i int, c explore.Category) {
				defer wg.Done()
				items, err := a.browseItems(ctx, r, src, c, sw, kid)
				res[i], errs[i] = explore.CategoryResult{Cat: c, Items: items}, err
			}(i, c)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil && ctx.Err() != nil {
				return nil, err
			}
		}
		tiles := explore.PickCategoryTiles(res, pg.Taste, b.used, b.names)
		for _, t := range tiles {
			b.names[t.Name] = true
		}
		b.sections[s.ID] = tiles
		b.next++
		if s.ID == id {
			return tiles, nil
		}
	}
	return []explore.CategoryTile{}, nil
}

// searchBrowseSection answers GET /api/v1/explore/browse/section?type=&section=:
// one Browse section's category tiles (Genres: the categories the page's
// genre tiles don't already cover). Each needs BrowseMin titles with a
// backdrop; no backdrop repeats across the tab's sections; a Kid gets
// kid-safe categories only.
func (a *API) searchBrowseSection(w http.ResponseWriter, r *http.Request) {
	sw, topKind, topRegion, ok := exploreParams(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("section")
	known := false
	for _, s := range explore.BrowseSections {
		known = known || s.ID == id
	}
	if !known {
		fail(w, http.StatusBadRequest, "section is not a Browse section")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	pg, page, kid, err := a.browsePage(r.Context(), r, sw, topKind, topRegion)
	if err != nil {
		internal(w, err)
		return
	}
	tiles, err := a.browseSectionTiles(r.Context(), r, sw, kid, pg, page, id)
	if err != nil {
		internal(w, err)
		return
	}
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "section": id, "tiles": tiles})
}

// searchBrowseCategory answers GET /api/v1/explore/browse/category?type=&id=:
// a Browse category's titles for the tab, most popular first.
func (a *API) searchBrowseCategory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := exploreSwitch(q.Get("type"))
	if !ok {
		fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	kid := a.kidMax(r) != ""
	c, found := explore.CategoryByID(q.Get("id"))
	if !found || (kid && !c.Kid) || len(c.Requests(sw, kid)) == 0 {
		fail(w, http.StatusNotFound, "no such Browse category")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	items, err := a.browseItems(r.Context(), r, a.liveSource(r.Context()), c, sw, kid)
	if err != nil {
		internal(w, err)
		return
	}
	if len(items) > browseCategoryMax {
		items = items[:browseCategoryMax]
	}
	views, err := a.exploreViews(r.Context(), r, []explore.Row{{ID: "browse-" + c.ID, Type: "browse", Title: c.Name, Items: items}})
	if err != nil {
		internal(w, err)
		return
	}
	out := []discoverItemView{}
	if len(views) > 0 && views[0].Items != nil {
		out = views[0].Items
	}
	a.exploreCaching(w, r, "private, no-cache")
	writeJSON(w, http.StatusOK, map[string]any{"type": sw, "id": c.ID, "name": c.Name, "section": c.Section, "items": out})
}
