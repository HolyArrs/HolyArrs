package sources

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
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
	versionID, partID, dlID, kind, err := mediafiles.SlotRetryFacts(ctx, db, slotID)
	if err != nil {
		return err
	}
	if kind == "artist" && partID.Valid {
		_, _, err := s.Queue.Enqueue(ctx, jobs.KindSearchAlbum, &domain.Subject{Type: "part", ID: partID.Int64},
			map[string]any{"version_id": versionID}, fmt.Sprintf("search_album:%d:%d", partID.Int64, versionID), by)
		return err
	}
	if kind == "show" && partID.Valid && dlID.Valid {
		same, err := slotsOfDownloadInSeason(ctx, db, dlID.Int64, partID.Int64, versionID)
		if err != nil {
			return err
		}
		if same > 1 {
			_, _, err := s.Queue.Enqueue(ctx, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: partID.Int64},
				map[string]any{"version_id": versionID}, fmt.Sprintf("search_season:%d:%d", partID.Int64, versionID), by)
			return err
		}
	}
	_, _, err = s.Queue.Enqueue(ctx, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: slotID}, nil, "search_slot:"+strconv.FormatInt(slotID, 10), by)
	return err
}
