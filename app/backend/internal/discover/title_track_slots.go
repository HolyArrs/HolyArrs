package discover

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// TrackSlots builds the Slot views for an album's tracks, keyed by item id.
func TrackSlots(ctx context.Context, db *sql.DB, p mediafiles.Part) (map[int64][]shows.EpisodeSlot, error) {
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", p.TitleID)
	if err != nil {
		return nil, err
	}
	label := map[int64]string{}
	for _, v := range vs {
		label[v.ID] = v.Label
	}
	slots, err := database.Select[mediafiles.Slot](ctx, db, "slots", "item_id IN (SELECT id FROM items WHERE part_id = ? AND kind = 'track') ORDER BY id", p.ID)
	if err != nil {
		return nil, err
	}
	byItem := map[int64][]shows.EpisodeSlot{}
	for _, s := range slots {
		es := shows.EpisodeSlot{ID: s.ID, Version: label[s.VersionID], VersionID: s.VersionID, State: string(s.State), StateLabel: mediafiles.StateLabels[s.State], FileID: s.FileID}
		if s.LastReason != nil {
			es.Reason = *s.LastReason
		}
		byItem[s.ItemID] = append(byItem[s.ItemID], es)
	}
	TrimEpisodeSlots(ctx, byItem)
	return byItem, nil
}

// TrimEpisodeSlots leaves out the Library-only details on episode and track
// Slots, as TrimForViewer does for a Title's Slots.
func TrimEpisodeSlots(ctx context.Context, slots map[int64][]shows.EpisodeSlot) {
	if SeesLibraryDetail(ctx) {
		return
	}
	for _, sl := range slots {
		for i := range sl {
			sl[i].Reason = ""
			sl[i].File = TrimFile(sl[i].File)
		}
	}
}
