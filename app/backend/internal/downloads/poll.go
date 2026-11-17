package downloads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// SyncOnce reads every active Download from its Downloader and records what
// changed, after asking again for any removal a Downloader missed (see
// retryRemovals) and any clear of a finished job whose wait is over (see
// retryClears). It returns how many Downloads are still active. With nothing
// active and nothing pending it writes nothing.
func (m *Manager) SyncOnce(ctx context.Context) (int, error) {
	m.retryRemovals(ctx)
	m.retryClears(ctx)
	m.relinkOrphans(ctx)
	db := m.Store.Read()
	dls, err := database.Select[domain.Download](ctx, db, "downloads", "state IN ("+activeStates+") ORDER BY id")
	if err != nil || len(dls) == 0 {
		return 0, err
	}
	byDownloader := map[int64][]domain.Download{}
	for _, d := range dls {
		byDownloader[d.DownloaderID] = append(byDownloader[d.DownloaderID], d)
	}
	st := m.settings(ctx)
	useStore := m.storeOn()
	active := 0
	var errs []error
	for did, list := range byDownloader {
		var dr domain.Downloader
		if err := database.Get(ctx, db, "downloaders", did, &dr); err != nil {
			errs = append(errs, err)
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var jobsIn []Job
		// fromStore: this tick reads the reader's snapshot taken at readAt.
		fromStore := false
		var readAt time.Time
		var v SyncView
		if useStore {
			v = m.Sync.View(dr.ID)
			switch {
			case v.Fresh:
				m.Sync.Fallback(dr.ID, false)
				jobsIn, fromStore, readAt = snapshotJobs(v, dr.Categories()), true, v.ReadAt
			case !v.Degraded:
				// Not fresh yet but within its bound: every row is unknown this tick, with no
				// state change and no stall timer change.
				active += len(list)
				continue
			default:
				m.Sync.Fallback(dr.ID, true)
			}
		}
		if !fromStore {
			jobsIn, err = listCategories(ctx, e, dr.Categories())
			if err != nil {
				m.downloaderFailed(ctx, dr, err)
				errs = append(errs, err)
				active += len(list)
				continue
			}
			m.downloaderOK(ctx, dr)
		}
		have := map[string]Job{}
		for _, j := range jobsIn {
			have[j.ID] = j
		}
		for _, d := range list {
			j, ok := have[d.ExternalID]
			if !ok {
				j, ok = outsideJob(ctx, e, dr, v, fromStore, d.ExternalID)
			}
			if fromStore && !ok && !readAt.After(d.UpdatedAt) {
				// Missing from a snapshot no newer than the row means unknown, so a grab the
				// client has not listed yet is never marked vanished.
				active++
				continue
			}
			if ok && d.PackFiles != nil && len(d.PackFiles) == 0 {
				// A season pack added stopped from a magnet: its files are chosen once the
				// client knows them, and it starts then.
				m.seen(d.ID)
				m.packPending(ctx, dr, d)
				active++
				continue
			}
			if ok {
				m.seen(d.ID)
			} else if !(d.State == "sent" && m.now().Sub(lastMoved(d)) < m.vanishGrace()) && !m.missedLongEnough(d.ID, readAt) {
				// Missing for a short while is not vanished yet.
				active++
				continue
			}
			if ok && j.State == StateFinished {
				if full, err := e.Get(ctx, d.ExternalID); err == nil {
					j = full
				}
				if len(j.Files) == 0 {
					j.SavePath, j.Files = readFinished(j, dr.PathMap, mapLocal(m.SavePath, dr.PathMap))
				}
			} else if ok && st.failDangerous && dr.Protocol == "torrent" && len(d.FilesSeen) == 0 && j.State != StateFailed {
				// Read the file list once, as soon as the torrent has it, so a fake fails
				// before it downloads.
				if full, err := e.Get(ctx, d.ExternalID); err == nil && len(full.Files) > 0 {
					j.Files = full.Files
				}
			}
			if m.apply(ctx, dr, d, j, ok, st) {
				active++
			}
		}
	}
	return active, errors.Join(errs...)
}

// Interval is how long to wait before the next sync.
func (m *Manager) Interval(active int) time.Duration {
	if active > 0 {
		if m.Active > 0 {
			return m.Active
		}
		return 2 * time.Second
	}
	if m.Idle > 0 {
		return m.Idle
	}
	return 30 * time.Second
}

// Poke makes the sync loop run now.
func (m *Manager) Poke() {
	m.mu.Lock()
	if m.wake == nil {
		m.wake = make(chan struct{}, 1)
	}
	w := m.wake
	m.mu.Unlock()
	select {
	case w <- struct{}{}:
	default:
	}
}

// Run syncs until ctx ends: every 2s while Downloads are active, 30s when idle,
// and it checks every Downloader once a minute, with the remove after seeding
// and dead torrent checks beside it.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	if m.wake == nil {
		m.wake = make(chan struct{}, 1)
	}
	w := m.wake
	m.mu.Unlock()
	every := m.HealthEvery
	if every <= 0 {
		every = time.Minute
	}
	var changed <-chan struct{}
	if m.Sync != nil {
		changed = m.Sync.Changed()
	}
	var checked time.Time
	for {
		if time.Since(checked) >= every {
			m.CheckAll(ctx)
			m.tidy(ctx)
			m.settleAttempts(ctx)
			m.settleLostChoices(ctx)
			checked = time.Now()
		}
		n, _ := m.SyncOnce(ctx)
		t := time.NewTimer(min(m.Interval(n), every))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-w:
			t.Stop()
		case <-changed:
			// A snapshot moved: apply it now. The floor timer still runs the time-based
			// checks when nothing moves.
			t.Stop()
		case <-t.C:
		}
	}
}

func (m *Manager) event(ctx context.Context, d *domain.Download, kind, summary string) {
	id := d.ID
	data := map[string]any{"state": d.State, "progress": d.Progress, "save_path": d.SavePath}
	if d.StateReason != nil {
		data["reason"] = *d.StateReason
	}
	db := m.Store.Read()
	var name string
	if d.ReleaseID != nil && db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *d.ReleaseID).Scan(&name) == nil {
		data["release"], data["release_id"] = name, *d.ReleaseID
	}
	if db.QueryRowContext(ctx, "SELECT name FROM downloaders WHERE id = ?", d.DownloaderID).Scan(&name) == nil {
		data["downloader"] = name
	}
	e := &domain.Event{Kind: kind, DownloadID: &id, Summary: summary, Data: data}
	if len(d.SlotIDs) > 1 {
		data["slot_ids"] = d.SlotIDs // a pack: Alerts name every episode
	}
	if len(d.SlotIDs) > 0 {
		s := d.SlotIDs[0]
		e.SlotID = &s
		e.GroupKey = fmt.Sprintf("slot:%d", s)
		var titleID int64
		if db.QueryRowContext(ctx, "SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", s).Scan(&titleID) == nil {
			e.TitleID = &titleID
		}
	}
	if m.Store.AddEvent(ctx, e) == nil {
		m.publish("event", e)
	}
}

// apply records one Download's new state and reports whether it is still active.
func (m *Manager) apply(ctx context.Context, dr domain.Downloader, d domain.Download, j Job, found bool, st settings) bool {
	now := m.now()
	next := d
	last := lastMoved(d)
	failReason := ""
	switch {
	case !found && d.State == "sent" && now.Sub(last) < m.vanishGrace():
		// Added by URL: the Downloader is still fetching the file.
		return true
	case !found:
		m.seen(d.ID)
		next.State = "vanished"
		next.StateReason = strPtr("removed outside HolyArrs: the Downloader no longer has it")
	case st.failDangerous && j.State != StateFailed && len(fakeFiles(j.Files, dr.Protocol)) > 0:
		// Known as soon as the Downloader lists the files, so a fake is not downloaded
		// to the end.
		next.State = "failed"
		failReason = fakeReason(j.Files, dr.Protocol)
		next.StateReason = &failReason
	case j.State == StateFinished:
		next.State, next.Progress = "finished", 1
		next.FinishedAt = &now
		next.Speed, next.UpSpeed, next.ETA = 0, 0, nil
	case j.State == StateFailed:
		next.State = "failed"
		failReason = clientSaysPrefix + j.Detail
		next.StateReason = &failReason
	default:
		next.Progress, next.Speed = min(max(j.Progress, 0), 1), j.Speed
		next.UpSpeed, next.Seeds, next.Peers = j.UpSpeed, j.Seeds, j.Peers
		if j.ETA > 0 {
			eta := j.ETA
			next.ETA = &eta
		} else {
			next.ETA = nil
		}
		// A higher mark restarts the stall timer. So does moving again after waiting or
		// being paused, which never count.
		waited := d.State == "queued" || d.State == "paused"
		if j.Progress > d.Progress || d.ProgressAt == nil || waited && j.State == StateDownloading {
			last = now
			next.ProgressAt = &now
		}
		quiet := now.Sub(last)
		switch {
		case j.State == StateQueued:
			next.State = "queued"
		case j.State == StatePaused:
			next.State = "paused"
			if cantUseFiles(j) {
				next.StateReason = strPtr(WaitingReason + " (" + j.Detail + "): " + WaitingHint)
			} else if d.StateReason != nil && strings.HasPrefix(*d.StateReason, WaitingReason) {
				next.StateReason = nil
			}
		case next.Progress >= 1:
			// Repairing or unpacking at full progress is never stalled.
			next.State = "downloading"
			next.StateReason = nil
		case quiet >= st.actAfter:
			next.State = "failed"
			failReason = fmt.Sprintf("no progress for %s", hours(quiet))
			next.StateReason = &failReason
		case quiet >= st.stallAfter:
			next.State = "stalled"
			next.StateReason = strPtr(fmt.Sprintf("no progress for %s", hours(quiet)))
		default:
			next.State = "downloading"
			next.StateReason = nil
		}
		if next.Progress < d.Progress {
			// Recorded, but only a higher mark resets the stall timer.
			next.Progress = d.Progress
		}
		if next.StartedAt == nil && next.State == "downloading" {
			next.StartedAt = &now
		}
	}
	if found {
		if j.SavePath != "" {
			next.SavePath = j.SavePath
		}
		if len(j.Files) > 0 {
			next.FilesSeen = make([]domain.SeenFile, len(j.Files))
			for i, f := range j.Files {
				next.FilesSeen[i] = domain.SeenFile{Name: f.Name, Size: f.Size, Progress: f.Progress, Priority: f.Priority}
			}
		}
	}
	if next.State == "failed" || next.State == "vanished" {
		m.failed(ctx, dr, d, next, st, domain.System)
		return false
	}
	if st.failDangerous && len(d.FilesSeen) == 0 && len(j.Files) > 0 {
		// A web shortcut beside the media is common on public torrents. Note it once,
		// when the file list is first seen.
		if sc := shortcuts(j.Files); len(sc) > 0 {
			m.event(ctx, &next, "download_shortcut", "Web shortcut in the download, left alone: "+strings.Join(sc, ", "))
		}
	}
	changed := next.State != d.State || next.Progress != d.Progress || next.Speed != d.Speed || next.UpSpeed != d.UpSpeed || next.Seeds != d.Seeds || next.Peers != d.Peers || !eqTime(next.ProgressAt, d.ProgressAt) ||
		!eqPtr(next.ETA, d.ETA) || !eqStr(next.StateReason, d.StateReason) || next.SavePath != d.SavePath || len(next.FilesSeen) != len(d.FilesSeen)
	if !changed {
		return isActive(next.State)
	}
	// The write holds only while the row is still in the state this snapshot read.
	// Mark failed (or another path) may have moved it since, and a stale snapshot is
	// dropped.
	// A season pack's files and Slots are read again in the same write, so a Widen
	// since the snapshot is kept.
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var cur domain.Download
		if err := database.Get(ctx, tx, "downloads", d.ID, &cur); err != nil {
			return err
		}
		next.PackFiles, next.SlotIDs = cur.PackFiles, cur.SlotIDs
		return database.UpdateWhere(tx, "downloads", &next, domain.System, "state = ?", d.State)
	}); err != nil {
		return !errors.Is(err, database.ErrNotFound)
	}
	m.publish("download", next)
	if next.State != d.State {
		m.stateMoved(ctx, d.State, &next)
	}
	return isActive(next.State)
}

func hours(d time.Duration) string {
	if d >= time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()))
}

func (m *Manager) stateMoved(ctx context.Context, from string, d *domain.Download) {
	m.event(ctx, d, "download_"+d.State, fmt.Sprintf("Download %s: %s", d.ExternalID[:min(8, len(d.ExternalID))], d.State))
	var to mediafiles.SlotState
	reason := ""
	if d.StateReason != nil {
		reason = *d.StateReason
	}
	switch d.State {
	case "stalled":
		to = mediafiles.SlotStalled
	case "downloading", "finished":
		// Back from stalled, or finished straight from stalled: the Slots wait for this
		// download again, so the import finds them.
		if from == "stalled" {
			to = mediafiles.SlotDownloading
		}
	}
	for _, sid := range d.SlotIDs {
		if to != "" {
			if err := mediafiles.MoveSlot(ctx, m.Store, sid, to, reason, domain.System); err == nil {
				m.publish("slot", map[string]any{"id": sid, "state": to, "reason": reason})
			}
		}
	}
	if d.State == "finished" && m.OnFinished != nil {
		m.OnFinished(ctx, d.ID)
	}
}

func isActive(state string) bool {
	switch state {
	case "sent", "queued", "downloading", "stalled", "paused":
		return true
	}
	return false
}

// vanishGrace is how long a sent Download may be missing from its Downloader
// before it counts as vanished.
func (m *Manager) vanishGrace() time.Duration {
	if m.VanishGrace > 0 {
		return m.VanishGrace
	}
	return 2 * time.Minute
}

func strPtr(s string) *string { return &s }

// unreachable reports whether err means the Downloader did not answer (a
// network error, a timeout, or an address the policy refused) or turned down the
// sign-in.
func unreachable(err error) bool {
	var ne net.Error
	var status adapterHTTPError
	return errors.Is(err, ErrAuth) || errors.Is(err, outbound.ErrSuppressed) || errors.Is(err, errDownloaderUnavailable) ||
		errors.As(err, &status) && (status.status == 408 || status.status == 429 || status.status >= 500) ||
		errors.Is(err, outbound.ErrAddressRefused) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne)
}

func eqPtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// lastMoved is when a Download last made progress, or its row last changed.
func lastMoved(d domain.Download) time.Time {
	if d.ProgressAt != nil {
		return *d.ProgressAt
	}
	return d.UpdatedAt
}

// cantUseFiles reports a paused job its client paused because it lost sight of
// the files: qBittorrent error, missingFiles or unknown, Deluge Error,
// Transmission local error 3, an rTorrent torrent closed with a storage error,
// an aria2 job stopped with an error.
func cantUseFiles(j Job) bool {
	if j.State != StatePaused {
		return false
	}
	switch j.Detail {
	case "error", "missingFiles", "unknown", "Error":
		return true
	}
	return strings.Contains(j.Detail, "; error 3") || strings.HasPrefix(j.Detail, "error: ")
}

func eqTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
