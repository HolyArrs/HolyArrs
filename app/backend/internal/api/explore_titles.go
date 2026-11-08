package api

import (
	"context"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
)

// The Explore title memory and counts live in internal/discover; these
// wrappers keep the api callers until their files move.

func exploreMoment(r *http.Request, now time.Time) (engine.Moment, string) {
	return discover.ExploreMoment(r, now)
}

func (a *API) titleExplore(ctx context.Context, member int64, pg engine.Page, m engine.Moment) (engine.Page, error) {
	return discover.TitleExplore(ctx, a.s.Store, member, pg, m)
}

func (a *API) exploreTitleCount(ctx context.Context, member int64, field string, layers []string) {
	discover.ExploreTitleCount(ctx, a.s.Store, a.exploreNow(), member, field, layers)
}

func validTitleLayer(l string) bool { return discover.ValidTitleLayer(l) }

func rowLayers(rows []exploreRowView) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TitleLayer)
	}
	return out
}

func (a *API) exploreTitleStats(w http.ResponseWriter, r *http.Request) {
	discover.ExploreTitleStats(w, r, a.s.Store, a.exploreNow())
}
