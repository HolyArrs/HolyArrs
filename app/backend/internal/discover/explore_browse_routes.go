package discover

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// browseTTL is how long built Browse sections are kept (the lists under
// them are cached by their own TTL, 12 h for discover).
const browseTTL = 6 * time.Hour

// browseRefreshAhead: the warm-up rebuilds a shared section this long
// before it expires, so members never meet an expired one.
const browseRefreshAhead = 45 * time.Minute

// browseFetchers is how many Browse categories are fetched at once, by the
// warm-up and members together (keeps the metadata server's limits and
// leaves room for everything else).
const browseFetchers = 3

// browseWarmGap is the warm-up's pause after starting each category (low
// priority: a member's build takes the free fetchers between them).
const browseWarmGap = 150 * time.Millisecond

// BrowseWait is how long a section request waits for a build before it
// answers "building".
var BrowseWait = 1500 * time.Millisecond

// browseCategoryMax is how many titles a category view shows.
const browseCategoryMax = 40

// browseFetch limits the Browse category fetches across the server.
var browseFetch = make(chan struct{}, browseFetchers)

// browseShared is one tab's fetched Browse categories for one rating limit,
// shared by every member with it (nothing personal: no taste, no backdrop
// order). Each section builds once at a time.
type browseShared struct {
	mu   sync.Mutex
	at   map[string]time.Time
	res  map[string][]engine.CategoryResult
	busy map[string]chan struct{}
}

var browseShares sync.Map

// browseShareKey: the shared cache is per tab, Kid flag and maximum rating.
func (h *Handlers) browseShareKey(sw string, kid bool, max string) string {
	return fmt.Sprintf("%p|%s|%t|%s", h.Explore, sw, kid, max)
}

func (h *Handlers) browseShare(sw string, kid bool, max string) *browseShared {
	v, _ := browseShares.LoadOrStore(h.browseShareKey(sw, kid, max), &browseShared{at: map[string]time.Time{}, res: map[string][]engine.CategoryResult{}, busy: map[string]chan struct{}{}})
	return v.(*browseShared)
}

// get is the section's results, whether it was built at all, and whether
// it is due a rebuild.
func (s *browseShared) get(id string) ([]engine.CategoryResult, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.res[id]
	return res, ok, !ok || time.Since(s.at[id]) >= browseTTL-browseRefreshAhead
}

// buildShared builds section id of the share (or waits for the build
// already running) and returns when it is done. A failed build keeps the
// section's earlier results.
func (h *Handlers) buildShared(ctx context.Context, s *browseShared, sw string, kid bool, max, id string, warm bool) {
	s.mu.Lock()
	if ch, busy := s.busy[id]; busy {
		s.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return
	}
	ch := make(chan struct{})
	s.busy[id] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.busy, id)
		s.mu.Unlock()
		close(ch)
	}()
	src := h.LiveExplore(ctx)
	cats := engine.SectionCategories(id, sw, kid, time.Now())
	res := make([]engine.CategoryResult, len(cats))
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i, c := range cats {
		select {
		case browseFetch <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(i int, c engine.Category) {
			defer wg.Done()
			defer func() { <-browseFetch }()
			items, err := h.browseItems(ctx, src, c, sw, kid, max)
			if err != nil {
				failed.Store(true)
			}
			if warm {
				// The warm-up learns the top backdrops' brightness first, so
				// the pickers skip too-dark art.
				SampleBackdrops(ctx, items, 4)
			}
			res[i] = engine.CategoryResult{Cat: c, Items: items}
		}(i, c)
		if warm {
			select {
			case <-time.After(browseWarmGap):
			case <-ctx.Done():
			}
		}
	}
	wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, had := s.res[id]; ctx.Err() != nil || (failed.Load() && had) {
		return
	}
	s.res[id], s.at[id] = res, time.Now()
}

// browseJob is one section of one tab for the warm-up.
type browseJob struct{ sw, section string }

// browseWarmOrder is the warm-up's work: Genres for every tab first, then
// each later section in page order.
func browseWarmOrder(sws []string) []browseJob {
	var out []browseJob
	for _, s := range engine.BrowseSections {
		for _, sw := range sws {
			out = append(out, browseJob{sw, s.ID})
		}
	}
	return out
}

// browseTabsOn are the tabs with a kind that is on.
func (h *Handlers) browseTabsOn() []string {
	var out []string
	for _, sw := range []string{"all", "movies", "shows"} {
		for _, k := range engine.Kinds(sw) {
			if len(h.KindsOn(k)) > 0 {
				out = append(out, sw)
				break
			}
		}
	}
	return out
}

// browseLimit is a rating limit the warm-up keeps warm.
type browseLimit struct {
	kid bool
	max string
}

// browseLimits are the open limit (always first) and every rating-limited
// share a member has opened.
func (h *Handlers) browseLimits() []browseLimit {
	out := []browseLimit{{false, ""}}
	prefix := fmt.Sprintf("%p|", h.Explore)
	browseShares.Range(func(k, _ any) bool {
		key := k.(string)
		if !strings.HasPrefix(key, prefix) {
			return true
		}
		p := strings.SplitN(strings.TrimPrefix(key, prefix), "|", 3)
		if len(p) != 3 {
			return true
		}
		l := browseLimit{p[1] == "true", p[2]}
		for _, x := range out {
			if x == l {
				return true
			}
		}
		out = append(out, l)
		return true
	})
	return out
}

// warmBrowseOnce builds every due section, in warm-up order.
func (h *Handlers) warmBrowseOnce(ctx context.Context) {
	for _, l := range h.browseLimits() {
		for _, j := range browseWarmOrder(h.browseTabsOn()) {
			if ctx.Err() != nil {
				return
			}
			s := h.browseShare(j.sw, l.kid, l.max)
			if _, _, due := s.get(j.section); due {
				h.buildShared(ctx, s, j.sw, l.kid, l.max, j.section, true)
			}
		}
	}
}

// warmBrowse builds every Browse section for every tab at start, then
// checks every 10 minutes and rebuilds those near expiry.
func (h *Handlers) warmBrowse(ctx context.Context) {
	for {
		h.warmBrowseOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}

// browseBuild is one member's Browse for one tab: the shared sections
// picked in order with the household's taste, so no backdrop repeats.
type browseBuild struct {
	mu       sync.Mutex
	at       time.Time
	used     map[string]bool
	names    map[string]bool
	next     int
	sections map[string][]engine.CategoryTile
}

var browseBuilds sync.Map

// sweepBrowseBuilds forgets builds older than browseTTL (all but keep), so
// the map does not grow with every member, switch and limit ever seen.
func sweepBrowseBuilds(keep any) {
	browseBuilds.Range(func(k, v any) bool {
		if k == keep {
			return true
		}
		b := v.(*browseBuild)
		b.mu.Lock()
		stale := time.Since(b.at) >= browseTTL
		b.mu.Unlock()
		if stale {
			browseBuilds.CompareAndDelete(k, v)
		}
		return true
	})
}

// browseStart resets b from the member's page: the page's genre tiles take
// their backdrops and names first.
func (b *browseBuild) start(page []engine.BrowseTile) {
	b.at, b.next = time.Now(), 0
	b.used, b.names, b.sections = map[string]bool{}, map[string]bool{}, map[string][]engine.CategoryTile{}
	for _, t := range page {
		b.used[t.Backdrop] = true
		b.names[t.Name] = true
	}
}

// browsePage is the member's filtered page and its fast genre tiles.
func (h *Handlers) browsePage(ctx context.Context, r *http.Request, sw string) (engine.Page, []engine.BrowseTile, bool, error) {
	pg, _, err := h.ExplorePageFor(ctx, r, h.UserID(r), sw)
	if err != nil {
		return pg, nil, false, err
	}
	if pg, err = h.genreRowPage(ctx, r, pg); err != nil {
		return pg, nil, false, err
	}
	kid := pg.Kid || h.KidMax(r) != ""
	return pg, engine.PageGenreTiles(pg, sw, kid, map[string]bool{}), kid, nil
}

// browseItems fetches category c for the tab (both kinds merged by
// popularity), without titles above the rating limit max. Never-add
// titles are left to browseDropNever.
func (h *Handlers) browseItems(ctx context.Context, src ExploreSource, c engine.Category, sw string, kid bool, max string) ([]engine.Item, error) {
	if c.ID == "awards" {
		if merged, ok, err := h.awardItems(ctx, src, c, sw, kid); ok {
			if err != nil || max == "" {
				return merged, err
			}
			return KeepItems(merged, h.KidItems(ctx, max, ItemRefs(merged), 0)), nil
		}
	}
	var lists [][]engine.Item
	for _, q := range c.Requests(sw, kid) {
		items, err := src.List(ctx, q)
		if err != nil {
			return nil, err
		}
		lists = append(lists, items)
	}
	merged := engine.MergeTop(lists, 80)
	if max == "" {
		return merged, nil
	}
	return KeepItems(merged, h.KidItems(ctx, max, ItemRefs(merged), 0)), nil
}

// browseDropNever leaves Never-add titles out of items.
func (h *Handlers) browseDropNever(ctx context.Context, items []engine.Item) ([]engine.Item, error) {
	refs, err := h.DropNeverAdd(ctx, ItemRefs(items))
	if err != nil {
		return nil, err
	}
	return KeepItems(items, refs), nil
}

// errBrowseBuilding: a section the member needs is still being built.
var errBrowseBuilding = errors.New("browse section building")

// browseSectionTiles picks the member's sections up to id (in order) from
// the shared ones and returns id's tiles. Shared sections not built yet
// build in the background; after BrowseWait the answer is
// errBrowseBuilding.
func (h *Handlers) browseSectionTiles(ctx context.Context, r *http.Request, sw string, kid bool, pg engine.Page, page []engine.BrowseTile, id string) ([]engine.CategoryTile, error) {
	max := h.KidMax(r)
	key := fmt.Sprintf("%p|%d|%s|%t|%s|%d", h.Explore, h.UserID(r), sw, kid, max, hiddenGen(h.Store))
	v, loaded := browseBuilds.LoadOrStore(key, &browseBuild{})
	if !loaded {
		sweepBrowseBuilds(key)
	}
	b := v.(*browseBuild)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used == nil || time.Since(b.at) >= browseTTL {
		b.start(page)
	}
	if got, ok := b.sections[id]; ok {
		return got, nil
	}
	share := h.browseShare(sw, kid, max)
	for b.next < len(engine.BrowseSections) {
		s := engine.BrowseSections[b.next]
		res, ok, _ := share.get(s.ID)
		if !ok {
			missing := engine.BrowseSections[b.next:]
			bg := context.WithoutCancel(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				for _, x := range missing {
					if _, have, _ := share.get(x.ID); !have {
						h.buildShared(bg, share, sw, kid, max, x.ID, false)
					}
					if x.ID == id {
						return
					}
				}
			}()
			select {
			case <-done:
			case <-time.After(BrowseWait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if res, ok, _ = share.get(s.ID); !ok {
				return nil, errBrowseBuilding
			}
		}
		picked := make([]engine.CategoryResult, 0, len(res))
		for _, c := range res {
			items, err := h.browseDropNever(ctx, c.Items)
			if err != nil {
				return nil, err
			}
			picked = append(picked, engine.CategoryResult{Cat: c.Cat, Items: items})
		}
		tiles := engine.PickCategoryTiles(picked, pg.Taste, b.used, b.names)
		for _, t := range tiles {
			b.names[t.Name] = true
		}
		b.sections[s.ID] = tiles
		b.next++
		if s.ID == id {
			return tiles, nil
		}
	}
	return []engine.CategoryTile{}, nil
}

// searchBrowseSection answers GET /api/v1/explore/browse/section?type=&section=:
// one Browse section's category tiles (Genres: the categories the page's
// genre tiles don't already cover). Each needs BrowseMin titles with a
// backdrop; no backdrop repeats across the tab's sections; a Kid gets
// kid-safe categories only.
func (h *Handlers) searchBrowseSection(w http.ResponseWriter, r *http.Request) {
	sw, ok := ExploreParams(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("section")
	known := false
	for _, s := range engine.BrowseSections {
		known = known || s.ID == id
	}
	if !known {
		webserver.Fail(w, http.StatusBadRequest, "section is not a Browse section")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	pg, page, kid, err := h.browsePage(r.Context(), r, sw)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	tiles, err := h.browseSectionTiles(r.Context(), r, sw, kid, pg, page, id)
	if errors.Is(err, errBrowseBuilding) {
		// Still building: the page shows ghost tiles and asks again.
		w.Header().Set("Cache-Control", "no-store")
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "section": id, "tiles": []engine.CategoryTile{}, "building": true})
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "section": id, "tiles": tiles})
}

// searchBrowseCategory answers GET /api/v1/explore/browse/category?type=&id=:
// a Browse category's titles for the tab, most popular first.
func (h *Handlers) searchBrowseCategory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, ok := ExploreSwitch(q.Get("type"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "type must be all, movies or shows")
		return
	}
	kid := h.KidMax(r) != ""
	c, found := engine.CategoryByID(q.Get("id"))
	if !found || (kid && !c.Kid) || len(c.Requests(sw, kid)) == 0 {
		webserver.Fail(w, http.StatusNotFound, "no such Browse category")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	items, err := h.browseItems(r.Context(), h.LiveExplore(r.Context()), c, sw, kid, h.KidMax(r))
	if err == nil {
		items, err = h.browseDropNever(r.Context(), items)
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if len(items) > browseCategoryMax {
		items = items[:browseCategoryMax]
	}
	views, err := h.ExploreViews(r.Context(), r, []engine.Row{{ID: "browse-" + c.ID, Type: "browse", Title: c.Name, Items: items}})
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.rowsText(r, views)
	out := []ItemView{}
	if len(views) > 0 && views[0].Items != nil {
		out = views[0].Items
	}
	h.Caching(w, r, "private, no-cache")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"type": sw, "id": c.ID, "name": c.Name, "section": c.Section, "items": out})
}
