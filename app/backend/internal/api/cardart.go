package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/discover/rows"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// cardArt is the wide card art filler (discover.CardArt) on this API's
// store and metadata provider.
func (a *API) cardArt() *discover.CardArt {
	return &a.explore.cards
}

// wireCardArt points the filler at this API's store and provider (New).
func (a *API) wireCardArt() {
	a.explore.cards.Store = func() *database.Store { return a.s.Store }
	a.explore.cards.Metadata = func() metadata.Provider { return a.s.Metadata }
}

func cardKey(kind, id, lang string) string { return discover.CardKey(kind, id, lang) }

func (a *API) readCardArt(ctx context.Context, keys [][2]string, lang string) map[string]discover.CardArtRow {
	return a.cardArt().Read(ctx, keys, lang)
}

func (a *API) fillCardArt(r *http.Request, items []discoverItemView) { a.cardArt().Fill(r, items) }

func (a *API) fillRowsCardArt(r *http.Request, rows []exploreRowView) {
	var all []discoverItemView
	for _, row := range rows {
		all = append(all, row.Items...)
	}
	a.fillCardArt(r, all)
	n := 0
	for i := range rows {
		copy(rows[i].Items, all[n:n+len(rows[i].Items)])
		n += len(rows[i].Items)
	}
}

func (a *API) noteCardLang(lang string) { a.cardArt().NoteLang(lang) }

func (a *API) libraryCardArt(r *http.Request, titles []mediafiles.Title) map[string]metadata.CardArt {
	return a.cardArt().Library(r, titles)
}

func (a *API) queueCardArt(ctx context.Context, pool *engine.Pool) { a.cardArt().QueuePool(ctx, pool) }

func (a *API) queueCardItems(ctx context.Context, all []engine.Item) {
	a.cardArt().QueueItems(ctx, all)
}
