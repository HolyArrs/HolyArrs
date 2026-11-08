package api

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

func withFreshRow(ctx context.Context, row string) context.Context {
	return discover.WithFreshRow(ctx, row)
}

// cardFresh is the card's one freshness strip (discover.CardFresh).
func (a *API) cardFresh(ctx context.Context, it metadata.DiscoverItem, titleID int64) (*engine.Fresh, error) {
	return discover.CardFresh(ctx, a.s.Store, a.explore.Now(), it, titleID)
}

// musicFresh is a music card's strip (discover.MusicFresh).
func (a *API) musicFresh(row string, c music.MusicCard) *engine.Fresh {
	return discover.MusicFresh(a.explore.Now(), row, c)
}
