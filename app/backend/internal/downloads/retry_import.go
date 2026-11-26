package downloads

import (
	"context"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// RetryImportFirst is the first step of a person's search on a title: each
// finished download that some of slotIDs wait on, because its import failed
// for a reason Import again can clear, has its import asked for again, as
// Sonarr retries the import of a completed download. It returns the Slots
// whose download was asked again; those are not searched, so a season pack
// already on disk is not fetched again episode by episode. Once the download
// is gone, nothing is retried and the search goes on as before.
func (m *Manager) RetryImportFirst(ctx context.Context, slotIDs []int64, by domain.Actor) map[int64]bool {
	db := m.Store.Read()
	byDownload := map[int64][]int64{}
	order := []int64{}
	for _, sid := range slotIDs {
		var sl mediafiles.Slot
		if mediafiles.LoadSlot(ctx, db, sid, &sl) != nil || sl.State != mediafiles.SlotNeedsADecision || sl.ActiveDownloadID == nil ||
			sl.LastReason == nil || !importer.CanImportAgain(*sl.LastReason) {
			continue
		}
		did := *sl.ActiveDownloadID
		if _, ok := byDownload[did]; !ok {
			order = append(order, did)
		}
		byDownload[did] = append(byDownload[did], sid)
	}
	done := map[int64]bool{}
	for _, did := range order {
		if m.ImportAgain(ctx, did, by) != nil {
			continue
		}
		for _, sid := range byDownload[did] {
			done[sid] = true
		}
	}
	return done
}
