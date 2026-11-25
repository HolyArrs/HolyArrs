package downloads

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// SettleGone settles at once the download a Slot follows when its Downloader
// no longer lists it, so a person picking another Release by hand is not told
// the Slot is still downloading while the usual misses add up. It acts only on
// a download the last sync already missed, and asks the Downloader for it by
// id first, as the sync does, so a job moved to another category is never
// taken for removed. It reports whether the download was settled.
func (m *Manager) SettleGone(ctx context.Context, slotID int64) bool {
	db := m.Store.Read()
	var sl mediafiles.Slot
	if err := database.Get(ctx, db, "slots", slotID, &sl); err != nil || sl.ActiveDownloadID == nil ||
		sl.State != mediafiles.SlotDownloading && sl.State != mediafiles.SlotStalled {
		return false
	}
	var d domain.Download
	if err := database.Get(ctx, db, "downloads", *sl.ActiveDownloadID, &d); err != nil || !isActive(d.State) {
		return false
	}
	m.mu.Lock()
	missed := m.missed[d.ID].n > 0
	m.mu.Unlock()
	if !missed {
		return false
	}
	var dr domain.Downloader
	if err := database.Get(ctx, db, "downloaders", d.DownloaderID, &dr); err != nil {
		return false
	}
	e, err := m.engine(ctx, dr)
	if err != nil {
		return false
	}
	if _, got := confirmGone(ctx, e, dr, SyncView{}, false, d.ExternalID); got != goneRemoved {
		return false
	}
	m.apply(ctx, dr, d, Job{}, false, m.settings(ctx), time.Time{})
	var now domain.Download
	return database.Get(ctx, m.Store.Read(), "downloads", d.ID, &now) == nil && now.State == "vanished"
}
