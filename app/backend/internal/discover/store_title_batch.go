package discover

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// titleBatch holds the movie Slots (with their Downloads and Files) and the
// asks of every Title, read in a few queries, so listing all Titles does
// not query them once per Title.
type titleBatch struct {
	slots     map[int64][]mediafiles.Slot // by title id (movies only)
	downloads map[int64]domain.Download
	files     map[int64]mediafiles.File
	labels    map[int64]mediafiles.Version
	asks      map[int64][]requests.View
}

type titleBatchKey struct{}

func batchOf(ctx context.Context) *titleBatch {
	b, _ := ctx.Value(titleBatchKey{}).(*titleBatch)
	return b
}

// WithTitleBatch reads the batch for all Titles and keeps it on ctx for View.
func (h *ViewHandlers) WithTitleBatch(ctx context.Context) (context.Context, error) {
	db := h.Store.Read()
	b := &titleBatch{slots: map[int64][]mediafiles.Slot{}, downloads: map[int64]domain.Download{}, files: map[int64]mediafiles.File{},
		labels: map[int64]mediafiles.Version{}, asks: map[int64][]requests.View{}}
	itemTitle := map[int64]int64{}
	rows, err := db.QueryContext(ctx, `SELECT i.id, i.title_id FROM items i JOIN titles t ON t.id = i.title_id WHERE t.kind = 'movie'`)
	if err != nil {
		return ctx, err
	}
	for rows.Next() {
		var item, title int64
		if err := rows.Scan(&item, &title); err != nil {
			rows.Close()
			return ctx, err
		}
		itemTitle[item] = title
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ctx, err
	}
	movieSlots := "item_id IN (SELECT i.id FROM items i JOIN titles t ON t.id = i.title_id WHERE t.kind = 'movie')"
	ss, err := database.Select[mediafiles.Slot](ctx, db, "slots", movieSlots+" ORDER BY id")
	if err != nil {
		return ctx, err
	}
	for _, s := range ss {
		b.slots[itemTitle[s.ItemID]] = append(b.slots[itemTitle[s.ItemID]], s)
	}
	ds, err := database.Select[domain.Download](ctx, db, "downloads", "id IN (SELECT active_download_id FROM slots WHERE "+movieSlots+")")
	if err != nil {
		return ctx, err
	}
	for _, d := range ds {
		b.downloads[d.ID] = d
	}
	fs, err := database.Select[mediafiles.File](ctx, db, "files", "id IN (SELECT file_id FROM slots WHERE "+movieSlots+")")
	if err != nil {
		return ctx, err
	}
	for _, f := range fs {
		b.files[f.ID] = f
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id IN (SELECT id FROM titles WHERE kind = 'movie')")
	if err != nil {
		return ctx, err
	}
	for _, v := range vs {
		b.labels[v.ID] = v
	}
	// Asks: the same rules as titleAsks, in one read.
	who, ok := webserver.From(ctx)
	var views []requests.View
	if !ok || who.Has(webserver.PermApprove) {
		views, err = h.Asks.Views(ctx, "a.title_id IS NOT NULL")
	} else {
		views, err = h.Asks.Views(ctx, "a.title_id IS NOT NULL AND EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)", who.UserID)
		for i := range views {
			views[i].Mine = true
		}
		requests.HideDeciderNames(views)
	}
	if err != nil {
		return ctx, err
	}
	for _, v := range views {
		if v.TitleID != nil {
			b.asks[*v.TitleID] = append(b.asks[*v.TitleID], v)
		}
	}
	return context.WithValue(ctx, titleBatchKey{}, b), nil
}

// asksFor answers a Title's asks from the batch (never nil, as Views).
func (b *titleBatch) asksFor(id int64) []requests.View {
	if v := b.asks[id]; v != nil {
		return v
	}
	return []requests.View{}
}
