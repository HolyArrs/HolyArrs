package discover

import (
	"context"
	"net/http"
	"time"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// The exported Browse pieces the Explore strip reads.

// BrowseShared is one tab's shared Browse sections for one rating limit.
type BrowseShared = browseShared

// ErrBrowseBuilding: a section the member needs is still being built.
var ErrBrowseBuilding = errBrowseBuilding

// BrowseFetch limits the Browse category fetches across the server.
var BrowseFetch = browseFetch

// Get is the section's results, whether it was built, and whether it is
// due a rebuild.
func (s *browseShared) Get(id string) ([]engine.CategoryResult, bool, bool) { return s.get(id) }

// Put keeps res as section id's results, built now.
func (s *browseShared) Put(id string, res []engine.CategoryResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.res[id], s.at[id] = res, time.Now()
}

// BrowsePage is the member's filtered page and its fast genre tiles.
func (h *Handlers) BrowsePage(ctx context.Context, r *http.Request, sw string) (engine.Page, []engine.BrowseTile, bool, error) {
	return h.browsePage(ctx, r, sw)
}

// BrowseShare is the shared sections for one tab and rating limit.
func (h *Handlers) BrowseShare(sw string, kid bool, max string) *BrowseShared {
	return h.browseShare(sw, kid, max)
}

// BuildShared builds section id of s (or waits for the running build).
func (h *Handlers) BuildShared(ctx context.Context, s *BrowseShared, sw string, kid bool, max, id string, warm bool) {
	h.buildShared(ctx, s, sw, kid, max, id, warm)
}

// BrowseSectionTiles is the member's tiles for section id.
func (h *Handlers) BrowseSectionTiles(ctx context.Context, r *http.Request, sw string, kid bool, pg engine.Page, page []engine.BrowseTile, id string) ([]engine.CategoryTile, error) {
	return h.browseSectionTiles(ctx, r, sw, kid, pg, page, id)
}

// BrowseItems fetches category c for the tab within the rating limit max.
func (h *Handlers) BrowseItems(ctx context.Context, src ExploreSource, c engine.Category, sw string, kid bool, max string) ([]engine.Item, error) {
	return h.browseItems(ctx, src, c, sw, kid, max)
}

// WarmBrowse keeps every Browse section built (runs until ctx ends).
func (h *Handlers) WarmBrowse(ctx context.Context) { h.warmBrowse(ctx) }

// SetBusy marks section id as building until ch closes (nil clears it).
func (s *browseShared) SetBusy(id string, ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch == nil {
		delete(s.busy, id)
		return
	}
	s.busy[id] = ch
}
