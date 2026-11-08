package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
)

// stripBrowse is the Browse glue the Explore strip (internal/discover)
// needs while Browse lives here.
func (a *API) stripBrowse() discover.StripBrowse {
	return discover.StripBrowse{
		Page: a.browsePage,
		Section: func(sw string, kid bool, max, id string) ([]engine.CategoryResult, bool) {
			res, built, _ := a.browseShare(sw, kid, max).Get(id)
			return res, built
		},
		Build: func(ctx context.Context, sw string, kid bool, max, id string) {
			a.buildShared(ctx, a.browseShare(sw, kid, max), sw, kid, max, id, false)
		},
		Tiles: func(ctx context.Context, r *http.Request, sw string, kid bool, pg engine.Page, page []engine.BrowseTile, id string) ([]engine.CategoryTile, error) {
			return a.browseSectionTiles(ctx, r, sw, kid, pg, page, id)
		},
		Building: errBrowseBuilding,
		Fetch:    browseFetch,
	}
}
