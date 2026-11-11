package discover

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// GenreRowPage drops the titles a row would not show this person: above a
// Kid's rating limit (max, "" for none), or marked never-add.
func GenreRowPage(ctx context.Context, pg engine.Page, max string,
	kidItems func(ctx context.Context, max string, items []metadata.DiscoverItem, n int) []metadata.DiscoverItem,
	dropNeverAdd func(ctx context.Context, items []metadata.DiscoverItem) ([]metadata.DiscoverItem, error),
) (engine.Page, error) {
	var all []metadata.DiscoverItem
	for _, rows := range [][]engine.Row{pg.Head, pg.Tail} {
		for _, row := range rows {
			for _, it := range row.Items {
				all = append(all, metadata.DiscoverItem{TMDBID: it.TMDBID, Kind: it.Kind, Genres: it.Genres})
			}
		}
	}
	if max != "" {
		all = kidItems(ctx, max, all, 0)
	}
	all, err := dropNeverAdd(ctx, all)
	if err != nil {
		return pg, err
	}
	ok := make(map[string]bool, len(all))
	for _, it := range all {
		ok[it.Kind+":"+it.TMDBID] = true
	}
	return pg.Filter(func(it engine.Item) bool { return ok[it.Key()] }), nil
}
