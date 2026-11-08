package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// rankedView is one ranked card of a top row.
type rankedView = discover.RankedView

func isCountry(r string) bool { return discover.IsCountry(r) }

func serviceKind(k string) string { return discover.ServiceKind(k) }

func (a *API) cachedServiceTop(ctx context.Context, region string) (metadata.ServiceTop, error) {
	return a.serviceTop.Get(ctx, region, a.s.ServiceTop)
}

func (a *API) warmServiceTop(ctx context.Context) { a.disc.WarmServiceTop(ctx) }

func (a *API) keptServiceTop(region, sw string) []engine.Item {
	return a.serviceTop.Kept(region, sw, discover.TopServicesMax)
}

type streamingMark = discover.StreamingMark

// fillStreaming marks the cards whose title is on one of the member's
// services (discover.Handlers.FillStreaming).
func (a *API) fillStreaming(r *http.Request, rows []exploreRowView) {
	items := make([][]discoverItemView, len(rows))
	for i := range rows {
		items[i] = rows[i].Items
	}
	a.disc.FillStreaming(r, items...)
}
