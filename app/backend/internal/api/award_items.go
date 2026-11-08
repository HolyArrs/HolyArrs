package api

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
)

// awardItems builds the Award winners collection (discover.AwardItems).
func (a *API) awardItems(ctx context.Context, src exploreSource, c engine.Category, sw string, kid bool) ([]engine.Item, bool, error) {
	return discover.AwardItems(ctx, src.List, a.titles.AwardWinners, c, sw, kid)
}
