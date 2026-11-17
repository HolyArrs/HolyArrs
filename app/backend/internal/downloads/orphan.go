package downloads

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrNotWaiting is returned by ImportAgain for a download with nothing left
// to import: not finished yet, or its Slots already moved on.
var ErrNotWaiting = errors.New("this download does not wait for an import")

// relinkEvery is how often one download of a removed Downloader is looked for
// in the other Downloaders, so a torrent nobody holds costs little.
const relinkEvery = 5 * time.Minute

// relinkMax caps the downloads looked for in one pass.
const relinkMax = 20

// orphan is a download whose Downloader was removed.
type orphan struct {
	id, downloader int64
	hash, state    string
}

// relinkOrphans moves the downloads of a removed Downloader to a Downloader
// that holds the same torrent, found by its hash, imported ones too.
// Removing a Downloader and adding it again otherwise leaves them on the
// removed one for good. A finished one whose import never ran has its
// import asked for again.
func (m *Manager) relinkOrphans(ctx context.Context) {
	if m.Store == nil {
		return
	}
	db := m.Store.Read()
	all, err := orphanCandidates(ctx, db)
	if err != nil {
		return
	}
	now := m.now()
	var due []orphan
	for _, o := range all {
		if len(due) >= relinkMax {
			break
		}
		if !validHash(o.hash) {
			continue
		}
		m.mu.Lock()
		if m.relinkTried == nil {
			m.relinkTried = map[int64]time.Time{}
		}
		last, ok := m.relinkTried[o.id]
		fresh := !ok || now.Sub(last) >= relinkEvery
		if fresh {
			m.relinkTried[o.id] = now
		}
		m.mu.Unlock()
		// The throttle comes first so a pile of old finished rows costs no reads each
		// sync. Imported ones move over too, so the torrent stays HolyArrs' own.
		if fresh {
			due = append(due, o)
		}
	}
	if len(due) == 0 {
		return
	}
	live, err := database.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0 AND protocol = 'torrent' ORDER BY priority, id")
	if err != nil || len(live) == 0 {
		return
	}
	for _, o := range due {
		for _, dr := range live {
			e, err := m.engine(ctx, dr)
			if err != nil {
				continue
			}
			if _, err := e.Get(ctx, o.hash); err != nil {
				continue
			}
			if m.relink(ctx, o, dr.ID) {
				break
			}
		}
	}
}

// relink moves one download to the Downloader to, unless a download of to
// already has that torrent. A finished one gets its waiting Slots back and
// its import asked for.
func (m *Manager) relink(ctx context.Context, o orphan, to int64) bool {
	var d domain.Download
	moved, woke := false, 0
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		ok, err := relinkTx(ctx, tx, o, to, m.now())
		if err != nil || !ok {
			return err
		}
		moved = true
		if err := database.Get(ctx, tx, "downloads", o.id, &d); err != nil {
			return err
		}
		if d.State == "finished" {
			woke, err = wakeSlots(ctx, tx, d, domain.System)
		}
		return err
	})
	if err != nil || !moved {
		return false
	}
	m.mu.Lock()
	delete(m.relinkTried, o.id)
	m.mu.Unlock()
	m.publish("download", d)
	// Only a download whose Slots still wait for it is imported; an imported one just moves.
	if d.State == "finished" && woke > 0 && m.OnFinished != nil {
		m.OnFinished(ctx, d.ID)
	}
	return true
}

// wakeSlots counts the Slots that still wait for d's files, putting a stalled
// one back to downloading so the import takes it.
func wakeSlots(ctx context.Context, tx *sql.Tx, d domain.Download, by domain.Actor) (int, error) {
	n := 0
	for _, sid := range d.SlotIDs {
		var sl mediafiles.Slot
		if err := mediafiles.LoadSlot(ctx, tx, sid, &sl); err != nil {
			continue
		}
		if sl.ActiveDownloadID != nil && *sl.ActiveDownloadID != d.ID {
			continue
		}
		switch sl.State {
		case mediafiles.SlotDownloading:
			n++
		case mediafiles.SlotStalled:
			if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotDownloading, "the download finished", by); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// ImportAgain asks again for the import of a finished download whose import
// never ran, so a row stuck before its import always has a way on.
func (m *Manager) ImportAgain(ctx context.Context, id int64, by domain.Actor) error {
	var d domain.Download
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "downloads", id, &d); err != nil {
			return err
		}
		if d.State != "finished" && d.State != "seeding" {
			return ErrNotWaiting
		}
		n, err := wakeSlots(ctx, tx, d, by)
		if err == nil && n == 0 {
			err = ErrNotWaiting
		}
		return err
	})
	if err != nil {
		return err
	}
	if m.OnFinished != nil {
		m.OnFinished(ctx, id)
	}
	return nil
}

// downloaderGone reports whether a download's Downloader was removed.
func (m *Manager) downloaderGone(ctx context.Context, id int64) (domain.Downloader, bool, error) {
	var dr domain.Downloader
	err := database.Get(ctx, m.Store.Read(), "downloaders", id, &dr)
	if errors.Is(err, database.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return dr, true, nil
	}
	return dr, err == nil && dr.Removed, err
}
