package downloads

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A download missing from its Downloader's list for vanishAfterMisses syncs and
// vanishMissingFor is asked for once by its id before it counts as removed, so
// a job the person moved to another category (Transmission labels, Deluge
// labels, rTorrent and SABnzbd/NZBGet categories, aria2 folders) is not taken
// for a removal and downloaded a second time. That one call is made only at
// that moment, never on every sync.

// emptyListWait is how long a Downloader that answers but lists none of
// HolyArrs's jobs, while HolyArrs follows more than one there, is taken for a
// client that is still starting or waiting for its share. Its jobs count as
// unknown until then; after that an empty list is a real removal.
const emptyListWait = time.Hour

// goneCheck is what asking a Downloader for a missing job by id found.
type goneCheck int

const (
	goneUnknown goneCheck = iota // the client did not answer clearly: keep following
	goneFound                    // it still has it in HolyArrs's categories
	goneRemoved                  // it no longer has it at all
	goneMoved                    // it has it in a category that is not HolyArrs's
)

// confirmGone asks once whether a job missing from the list of HolyArrs's
// categories is really gone. A snapshot of the whole client answers without a
// call.
func confirmGone(ctx context.Context, e Engine, dr domain.Downloader, v SyncView, fromStore bool, id string) (Job, goneCheck) {
	if id == "" {
		return Job{}, goneRemoved
	}
	cats := dr.Categories()
	if fromStore && v.Scope == "" {
		j, ok := v.Items[id]
		switch {
		case !ok:
			return Job{}, goneRemoved
		case ours(cats, j.Category) || dr.Adapter == "qbittorrent":
			return j, goneFound
		}
		return j, goneMoved
	}
	j, err := e.Get(ctx, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return Job{}, goneRemoved
	case errors.Is(err, errTransmissionForeign):
		return Job{}, goneMoved
	case err != nil:
		return Job{}, goneUnknown
	case ours(cats, j.Category) || dr.Adapter == "qbittorrent":
		// qBittorrent follows a job by its hash in any category (see outsideJob).
		return j, goneFound
	}
	return j, goneMoved
}

// ours reports whether category c is one HolyArrs follows on the Downloader.
// A job whose client reports no category is taken as still HolyArrs's.
func ours(cats []string, c string) bool {
	return c == "" || slices.Contains(cats, "") || hasCategory(cats, c)
}

// clientLooksEmpty reports whether a Downloader that answered lists none of the
// jobs HolyArrs follows there, with more than one followed, for less than
// emptyListWait: a client that is starting up or waiting for its share, not
// one where the person removed everything. A list with any job resets it.
func (m *Manager) clientLooksEmpty(downloaderID int64, listed, followed int) bool {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if listed > 0 || followed < 2 {
		delete(m.emptySince, downloaderID)
		return false
	}
	if m.emptySince == nil {
		m.emptySince = map[int64]time.Time{}
	}
	at, ok := m.emptySince[downloaderID]
	if !ok {
		at = now
		m.emptySince[downloaderID] = at
	}
	return now.Sub(at) < emptyListWait
}

// movedOut ends a download the person moved to a category that is not
// HolyArrs's in the Downloader. The job is theirs now: the row leaves the
// Downloads list, its Slots go back to wanted and nothing is searched again,
// so the same Release is not downloaded twice. The client is not touched.
func (m *Manager) movedOut(ctx context.Context, dr domain.Downloader, d domain.Download) {
	reason := MovedOutReason(dr.Name)
	next := d
	now := m.now()
	next.State, next.StateReason, next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = "cleaned", &reason, 0, 0, 0, 0, nil
	next.CleanedAt = &now
	rest := map[int64]mediafiles.SlotState{}
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		cur, err := downloadStateTx(tx, d.ID)
		if err != nil {
			return err
		}
		if cur != d.State || !(isActive(cur) || finishedState(cur)) {
			return ErrNotActive
		}
		if err := database.Update(tx, "downloads", &next, domain.System); err != nil {
			return err
		}
		for _, sid := range next.SlotIDs {
			state, active, file, err := mediafiles.SlotDownloadTx(tx, sid)
			if err != nil {
				continue
			}
			if !active.Valid || active.Int64 != d.ID || (state != string(mediafiles.SlotDownloading) && state != string(mediafiles.SlotStalled)) {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotFailed, reason, domain.System); err != nil {
				return err
			}
			to := mediafiles.SlotWanted
			if file.Valid {
				to = mediafiles.SlotBetterWanted
			}
			if err := mediafiles.MoveSlotTx(tx, sid, to, reason, domain.System); err != nil {
				return err
			}
			rest[sid] = to
		}
		return nil
	})
	if err != nil {
		return
	}
	m.seen(d.ID)
	m.publish("download", next)
	m.eventWith(ctx, &next, "download_removed", upperFirst(reason), map[string]any{"moved_out": true})
	for sid, to := range rest {
		m.publish("slot", map[string]any{"id": sid, "state": to, "reason": reason})
	}
}

// finishedGone ends a finished download whose job the person removed in the
// Downloader after its import: it leaves the list as cleaned, with no failure
// and no new search, since the files are already in the Library.
func (m *Manager) finishedGone(ctx context.Context, dr domain.Downloader, d domain.Download) {
	reason := VanishedReason(dr.Name)
	next := d
	now := m.now()
	next.State, next.StateReason, next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = "cleaned", &reason, 0, 0, 0, 0, nil
	next.CleanedAt = &now
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		return database.UpdateWhere(tx, "downloads", &next, domain.System, "state IN ('finished', 'seeding') AND removal_pending IS NULL AND clear_retry_at IS NULL")
	})
	if err != nil {
		return
	}
	m.seen(d.ID)
	m.publish("download", next)
	m.eventWith(ctx, &next, "download_removed", upperFirst(reason), map[string]any{"outside": true})
}

// MovedOutReason is the reason of a download the person moved out of
// HolyArrs's categories in the Downloader.
func MovedOutReason(downloader string) string {
	if downloader == "" {
		downloader = "the Downloader"
	}
	return "moved out of HolyArrs's category in " + downloader
}

// checkFinished looks for finished torrents the person removed in the
// Downloader, against a list this sync already read: no extra call unless one
// has been missing long enough to be asked for by id once.
func (m *Manager) checkFinished(ctx context.Context, e Engine, dr domain.Downloader, rows []domain.Download, have map[string]Job, v SyncView, fromStore bool, readAt time.Time) {
	for _, d := range rows {
		if _, ok := have[d.ExternalID]; ok {
			m.seen(d.ID)
			continue
		}
		if fromStore && !readAt.After(d.UpdatedAt) {
			continue
		}
		if !m.missedLongEnough(d.ID, readAt) {
			continue
		}
		switch _, got := confirmGone(ctx, e, dr, v, fromStore, d.ExternalID); got {
		case goneFound:
			m.seen(d.ID)
			continue
		case goneUnknown:
			continue
		case goneMoved:
			// The person took the seeding torrent over in another category.
			if ok, err := mediafiles.ImportSettled(ctx, m.Store.Read(), d.ID, d.SlotIDs); err == nil && ok {
				m.movedOut(ctx, dr, d)
			}
			continue
		}
		// Only once the import is done: an import still reading the files ends on
		// its own.
		if ok, err := mediafiles.ImportSettled(ctx, m.Store.Read(), d.ID, d.SlotIDs); err == nil && ok {
			m.finishedGone(ctx, dr, d)
		}
	}
}
