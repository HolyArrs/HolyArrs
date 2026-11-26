package downloads

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
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
			woke, _, err = wakeSlots(ctx, tx, d, domain.System, nil)
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
// one back to downloading so the import takes it. With retry, a Slot that
// waits for a person because the files could not be seen, read or written
// goes back to importing: the person asked for the import again. It keeps
// its reason meanwhile, and again holds each such Slot with that reason.
func wakeSlots(ctx context.Context, tx *sql.Tx, d domain.Download, by domain.Actor, retry func(reason string) bool) (n int, again map[int64]string, err error) {
	again = map[int64]string{}
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
				return n, again, err
			}
			n++
		case mediafiles.SlotNeedsADecision:
			if retry == nil || sl.LastReason == nil || !retry(*sl.LastReason) {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotImporting, *sl.LastReason, by); err != nil {
				return n, again, err
			}
			again[sid] = *sl.LastReason
			n++
		}
	}
	return n, again, nil
}

// rereadTimeout is how long Import again waits for the Downloader and the
// folder before it says the read took too long.
var rereadTimeout = 3 * time.Minute

// rereadWait is how long the click waits for the read; a slower read goes on
// in the background and the click says the import is queued.
var rereadWait = 5 * time.Second

// rereadFolder walks the folder of a finished job for Import again.
var rereadFolder = readFinishedIn

// slowReadWhy is the reason of a card whose folder read took too long.
const slowReadWhy = importer.SlowReadWhy

// ImportAgain asks again for the import of a finished download whose import
// never ran, so a row stuck before its import always has a way on. A page
// that goes away meanwhile changes nothing: the Slots it wakes always get
// their import queued, or go back to the card they were. A second click
// while the folder of the same download is read starts no second read.
func (m *Manager) ImportAgain(ctx context.Context, id int64, by domain.Actor) error {
	return m.importAgainWith(ctx, id, by, importer.CanImportAgain)
}

// importAgainWith is Import again for the Slots whose reason retry accepts;
// the other Slots of the download keep their cards.
func (m *Manager) importAgainWith(ctx context.Context, id int64, by domain.Actor, retry func(reason string) bool) error {
	ctx = context.WithoutCancel(ctx)
	if err := m.waitsForImport(ctx, id, retry); err != nil {
		return err
	}
	m.mu.Lock()
	if m.rereading[id] {
		m.mu.Unlock()
		return nil
	}
	if m.rereading == nil {
		m.rereading = map[int64]bool{}
	}
	m.rereading[id] = true
	m.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.rereading, id)
			m.mu.Unlock()
			// The read is over however it ended: open pages fetch the card again.
			var d domain.Download
			if database.Get(ctx, m.Store.Read(), "downloads", id, &d) == nil {
				for _, sid := range d.SlotIDs {
					m.publish("slot", map[string]any{"id": sid})
				}
			}
		}()
		err := m.importAgain(ctx, id, by, retry)
		done <- err
		if err != nil && !errors.Is(err, ErrNotWaiting) {
			log.Printf("downloads: import again of download %d: %v", id, err)
		}
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(rereadWait):
		return nil
	}
}

// Rereading reports whether Import again reads the folder of download id now.
func (m *Manager) Rereading(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rereading[id]
}

// waitsForImport is the quick check before any read: the download is there,
// finished, and has a Slot that still waits for its files or for Import again.
func (m *Manager) waitsForImport(ctx context.Context, id int64, retry func(reason string) bool) error {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", id, &d); err != nil {
		return err
	}
	if d.State != "finished" && d.State != "seeding" {
		return ErrNotWaiting
	}
	for _, sid := range d.SlotIDs {
		var sl mediafiles.Slot
		if mediafiles.LoadSlot(ctx, m.Store.Read(), sid, &sl) != nil || (sl.ActiveDownloadID != nil && *sl.ActiveDownloadID != d.ID) {
			continue
		}
		switch sl.State {
		case mediafiles.SlotDownloading, mediafiles.SlotStalled:
			return nil
		case mediafiles.SlotNeedsADecision:
			if sl.LastReason != nil && retry(*sl.LastReason) {
				return nil
			}
		}
	}
	return ErrNotWaiting
}

func (m *Manager) importAgain(ctx context.Context, id int64, by domain.Actor, retry func(reason string) bool) error {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", id, &d); err != nil {
		return err
	}
	if d.State != "finished" && d.State != "seeding" {
		return ErrNotWaiting
	}
	// Before any Slot is woken, so a slow Downloader holds no Slot in importing.
	// A mount stuck inside one file-system call ignores the limit, so the wait
	// ends on its own timer; a walk that answers later stores nothing.
	// A walk still stuck from an earlier press is not joined by a second one:
	// the press goes straight to the slow-read card.
	rctx, cancel := context.WithTimeout(ctx, rereadTimeout)
	var gaveUp atomic.Bool
	m.mu.Lock()
	stuck := m.walking[id]
	if !stuck {
		if m.walking == nil {
			m.walking = map[int64]bool{}
		}
		m.walking[id] = true
	}
	m.mu.Unlock()
	if stuck {
		gaveUp.Store(true)
	} else {
		read := make(chan struct{})
		go func() {
			defer close(read)
			defer func() {
				m.mu.Lock()
				delete(m.walking, id)
				m.mu.Unlock()
			}()
			m.readFilesAgain(rctx, d, &gaveUp)
		}()
		timer := time.NewTimer(rereadTimeout)
		select {
		case <-read:
		case <-timer.C:
			gaveUp.Store(true)
		}
		timer.Stop()
	}
	slow := rctx.Err() != nil || gaveUp.Load()
	cancel()
	var again map[int64]string
	var others int
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "downloads", id, &d); err != nil {
			return err
		}
		if d.State != "finished" && d.State != "seeding" {
			return ErrNotWaiting
		}
		n, woke, err := wakeSlots(ctx, tx, d, by, retry)
		if err == nil && n == 0 {
			err = ErrNotWaiting
		}
		again, others = woke, n-len(woke)
		return err
	})
	if err != nil {
		return err
	}
	// The read was cut short: the card says so instead of a folder that is not there.
	if slow && len(d.FilesSeen) == 0 && len(again) > 0 {
		for sid := range again {
			again[sid] = slowReadWhy
		}
		m.backToCards(ctx, again, by)
		if others == 0 {
			return nil
		}
		again = nil
	}
	if m.QueueImport != nil {
		if err := m.QueueImport(ctx, id); err != nil {
			m.backToCards(ctx, again, by)
			return err
		}
		return nil
	}
	if m.OnFinished != nil {
		m.OnFinished(ctx, id)
	}
	return nil
}

// backToCards puts the Slots an import was asked for again back to waiting
// for a person, each with the reason it had.
func (m *Manager) backToCards(ctx context.Context, again map[int64]string, by domain.Actor) {
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		for sid, reason := range again {
			if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotNeedsADecision, reason, by); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("downloads: putting Slots back to their card: %v", err)
	}
}

// readFilesAgain reads the files of a finished download that has no file
// list, the way the first read did, and stores them: the folder may be there
// now that a path pair, a mount or a permission was fixed. With nothing to
// read it changes nothing.
func (m *Manager) readFilesAgain(ctx context.Context, d domain.Download, gaveUp *atomic.Bool) {
	if len(d.FilesSeen) > 0 {
		return
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil || dr.Removed {
		return
	}
	e, err := m.engine(ctx, dr)
	if err != nil {
		return
	}
	j, err := e.Get(ctx, d.ExternalID)
	// A job the Downloader has not finished may hold half-written files.
	if err != nil || j.State != StateFinished {
		return
	}
	if len(j.Files) == 0 {
		j.SavePath, j.Files = rereadFolder(ctx, j, dr.PathMap, mapLocal(m.SavePath, dr.PathMap))
	}
	// A read its time limit cut short may hold half the files: none are stored.
	if len(j.Files) == 0 || ctx.Err() != nil {
		return
	}
	seen := make([]domain.SeenFile, len(j.Files))
	for i, f := range j.Files {
		seen[i] = domain.SeenFile{Name: f.Name, Size: f.Size, Progress: f.Progress, Priority: f.Priority}
	}
	// The read finished in time: what it found is stored even if the limit passes now.
	ctx = context.WithoutCancel(ctx)
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		// Checked inside the write, so it never lands after Import again gave up.
		if gaveUp.Load() {
			return nil
		}
		var now domain.Download
		if err := database.Get(ctx, tx, "downloads", d.ID, &now); err != nil || len(now.FilesSeen) > 0 {
			return err
		}
		if j.SavePath != "" {
			now.SavePath = j.SavePath
		}
		now.FilesSeen = seen
		return database.Update(tx, "downloads", &now, domain.System)
	})
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
