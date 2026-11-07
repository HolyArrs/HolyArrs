package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// QueueAfterFailure queues the search that follows a failed Download, for one of its Slots.
// A Download that filled several episodes of one season gets one search for that season and Version,
// because a pack may answer it again. Any other Slot gets its own search.
// Several calls for the Slots of one pack queue the season search once, through the job key.
func (s *Searcher) QueueAfterFailure(ctx context.Context, slotID int64, by domain.Actor) error {
	if s.Queue == nil {
		return errors.New("searching again needs the job queue")
	}
	db := s.Store.Read()
	var versionID int64
	var partID, dlID sql.NullInt64
	var kind string
	err := db.QueryRowContext(ctx, `SELECT s.version_id, i.part_id, s.active_download_id, t.kind FROM slots s JOIN items i ON i.id = s.item_id
		JOIN titles t ON t.id = i.title_id WHERE s.id = ?`, slotID).Scan(&versionID, &partID, &dlID, &kind)
	if err != nil {
		return err
	}
	if kind == "artist" && partID.Valid {
		_, _, err := s.Queue.Enqueue(ctx, KindSearchAlbum, &domain.Subject{Type: "part", ID: partID.Int64},
			map[string]any{"version_id": versionID}, fmt.Sprintf("search_album:%d:%d", partID.Int64, versionID), by)
		return err
	}
	if kind == "show" && partID.Valid && dlID.Valid {
		var same int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM downloads d, json_each(d.slot_ids) j JOIN slots s ON s.id = j.value
			JOIN items i ON i.id = s.item_id WHERE d.id = ? AND i.part_id = ? AND s.version_id = ?`, dlID.Int64, partID.Int64, versionID).Scan(&same); err != nil {
			return err
		}
		if same > 1 {
			_, _, err := s.Queue.Enqueue(ctx, KindSearchSeason, &domain.Subject{Type: "part", ID: partID.Int64},
				map[string]any{"version_id": versionID}, fmt.Sprintf("search_season:%d:%d", partID.Int64, versionID), by)
			return err
		}
	}
	_, _, err = s.Queue.Enqueue(ctx, KindSearchSlot, &domain.Subject{Type: "slot", ID: slotID}, nil, "search_slot:"+strconv.FormatInt(slotID, 10), by)
	return err
}
