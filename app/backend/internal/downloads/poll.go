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
	if err != nil {
		return 0, err
	}
	useStore := m.storeOn()
	finished := finishedRows(ctx, db)
	if len(dls) == 0 {
		// Nothing active: finished torrents are checked only against a fresh
		// snapshot, which costs no client call.
		if useStore {
			m.finishedFromSnapshots(ctx, db, finished)
		}
		return 0, nil
	}
	byDownloader := map[int64][]domain.Download{}
	for _, d := range dls {
		byDownloader[d.DownloaderID] = append(byDownloader[d.DownloaderID], d)
	}
	if useStore {
		rest := map[int64][]domain.Download{}
		for did, rows := range finished {
			if _, ok := byDownloader[did]; !ok {
				rest[did] = rows
			}
		}
		m.finishedFromSnapshots(ctx, db, rest)
	}
	st := m.settings(ctx)
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
		have := jobIndex(jobsIn)
		listed := len(jobsIn)
		if fromStore {
			listed = len(v.Items)
		}
		// An empty list from a client that answers is a real removal, except for a
		// while after it starts or waits for its share (see clientLooksEmpty).
		empty := m.clientLooksEmpty(dr.ID, listed, len(list)+len(finished[dr.ID]))
		dst := st
		dst.downloaderStuck = downloaderStuck(list, have, st.stallAfter, m.now())
		// restarted is when the client started again, if that is known: it then forgot
		// every download it was given, so one it does not list was not removed by hand.
		// Asked for once, and only when a download is missing.
		var restarted time.Time
		asked := false
		for _, d := range list {
			j, ok := have[d.ExternalID]
			if !ok {
				j, ok = outsideJob(ctx, e, dr, v, fromStore, d.ExternalID)
			}
			if !ok && !asked {
				asked, restarted = true, m.restartedAt(ctx, e, dr.ID)
			}
			if fromStore && !ok && !readAt.After(d.UpdatedAt) || !ok && empty && !forgotSince(d, restarted) {
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
				d = m.adoptID(ctx, d, j)
			} else if !(d.State == "sent" && m.now().Sub(lastMoved(d)) < m.vanishGrace()) {
				if !m.missedLongEnough(d.ID, readAt) {
					// Missing for a short while is not vanished yet.
					active++
					continue
				}
				// Missing long enough: ask for it by id once before calling it removed.
				found, got := confirmGone(ctx, e, dr, v, fromStore, d.ExternalID)
				switch got {
				case goneUnknown:
					active++
					continue
				case goneMoved:
					m.movedOut(ctx, dr, d)
					continue
				case goneFound:
					m.seen(d.ID)
					j, ok = found, true
				}
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
			if m.apply(ctx, dr, d, j, ok, dst, restarted) {
				active++
			}
		}
		if rows := finished[dr.ID]; len(rows) > 0 && !empty {
			m.checkFinished(ctx, e, dr, rows, have, v, fromStore, readAt)
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
			m.settleImported(ctx)
			m.tidy(ctx)
			m.settleAttempts(ctx)
			m.settleLostChoices(ctx)
			m.retryWaitingImports(ctx)
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
	m.eventWith(ctx, d, kind, summary, nil)
}

// eventWith is event with extra data keys for the History row.
func (m *Manager) eventWith(ctx context.Context, d *domain.Download, kind, summary string, extra map[string]any) {
	id := d.ID
	data := map[string]any{"state": d.State, "progress": d.Progress, "save_path": d.SavePath}
	for k, v := range extra {
		data[k] = v
	}
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
// restarted is when the Downloader's client started again, zero if unknown; a
// Download last seen before that was forgotten by the client, not removed.
func (m *Manager) apply(ctx context.Context, dr domain.Downloader, d domain.Download, j Job, found bool, st settings, restarted time.Time) bool {
	now := m.now()
	next := d
	last := lastMoved(d)
	failReason := ""
	forgot := forgotSince(d, restarted)
	switch {
	case !found && d.State == "sent" && now.Sub(last) < m.vanishGrace():
		// Added by URL: the Downloader is still fetching the file.
		return true
	case !found && forgot:
		// The client started again and lost what it was given. The row still goes, since
		// the download is not there to follow, but the reason is not a removal.
		m.seen(d.ID)
		next.State = "vanished"
		next.StateReason = strPtr(ForgotReason(dr.Name))
		next.CleanedAt = &now
	case !found:
		m.seen(d.ID)
		// Removed in the Downloader itself: the row leaves the Downloads list
		// (only History keeps it), and cleaned_at starts the same short wait as a
		// removal in HolyArrs before a job re-added by hand shows as an outside row.
		next.State = "vanished"
		next.StateReason = strPtr(VanishedReason(dr.Name))
		next.CleanedAt = &now
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
		case st.downloaderStuck && quiet >= st.stallAfter:
			// Every download on this Downloader stopped together: most likely its
			// connection (a VPN that died quietly), not bad Releases. The clock is held
			// at the stall mark, so nothing fails while it lasts, and it runs on from
			// there once any of them moves again.
			next.State = "stalled"
			next.StateReason = strPtr(StuckReason)
			if quiet-st.stallAfter >= time.Minute {
				held := now.Add(-st.stallAfter)
				next.ProgressAt = &held
			}
		case quiet >= st.actAfter:
			next.State = "failed"
			failReason = StallPrefix + hours(quiet)
			next.StateReason = &failReason
		case quiet >= st.stallAfter:
			next.State = "stalled"
			next.StateReason = strPtr(StallPrefix + hours(quiet))
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

// StallPrefix starts the reason of a download that stalled. A stall alone never
// turns the Release down or deletes its files: the Downloader or its VPN may be
// offline, so the next search may pick the same Release again and resume.
const StallPrefix = mediafiles.StallReasonPrefix

// StuckReason is the stall reason while every download on a Downloader stopped
// together. Health matches it word for word (health.downloaders).
const StuckReason = "every download on this Downloader stopped at the same time; check its internet connection (a VPN, for example)"

// downloaderStuck reports that at least two downloads on one Downloader are
// downloading and none of them moved for the stall time: the Downloader's own
// connection, not the Releases. Waiting, paused, complete and unlisted rows
// don't count either way; one row that moved means it is not the connection.
func downloaderStuck(list []domain.Download, have map[string]Job, stallAfter time.Duration, now time.Time) bool {
	n := 0
	for _, d := range list {
		j, ok := have[d.ExternalID]
		if !ok || d.State == "queued" || d.State == "paused" || d.State == "sent" || d.Progress >= 1 {
			continue
		}
		if j.State != StateDownloading || j.Progress >= 1 {
			continue
		}
		if j.Progress == 0 && d.Progress == 0 && (d.ProgressAt == nil || now.Sub(*d.ProgressAt) < stallAfter) {
			// A fresh grab that never started (the next one sent in while the
			// connection is down) proves nothing either way.
			continue
		}
		if j.Progress > d.Progress || d.ProgressAt == nil || now.Sub(*d.ProgressAt) < stallAfter {
			return false
		}
		n++
	}
	return n >= 2
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
	if d.State == "finished" && d.ReleaseID != nil {
		// It worked: a later stall of this Release starts its wait from 1 day again.
		var rel domain.Release
		if database.Get(ctx, m.Store.Read(), "releases", *d.ReleaseID, &rel) == nil {
			_ = mediafiles.ClearStall(ctx, m.Store, rel.Name)
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

// restartedAt is when the Downloader's client last started again, zero when it
// has not been seen to. A client that restarts forgets the jobs it was given, so
// a download it no longer lists was not removed by hand. Only a client that names
// its own run can say (aria2 does).
func (m *Manager) restartedAt(ctx context.Context, e Engine, id int64) time.Time {
	s, ok := e.(Sessioner)
	if !ok {
		return time.Time{}
	}
	session, err := s.Session(ctx)
	if err != nil || session == "" {
		return time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions, m.restarts = map[int64]string{}, map[int64]time.Time{}
	}
	before, seen := m.sessions[id]
	m.sessions[id] = session
	if seen && before != session {
		m.restarts[id] = m.now()
	}
	return m.restarts[id]
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
// forgotSince says the client started again after the download last moved, so a
// download it no longer lists was lost by that restart, not removed by a person.
func forgotSince(d domain.Download, restarted time.Time) bool {
	return !restarted.IsZero() && restarted.After(lastMoved(d))
}

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

// finishedFromSnapshots checks finished torrents on Downloaders with nothing
// active against their reader's fresh snapshot. A Downloader without one is
// left for a later sync: this costs no client call.
func (m *Manager) finishedFromSnapshots(ctx context.Context, db *sql.DB, rows map[int64][]domain.Download) {
	for did, list := range rows {
		v := m.Sync.View(did)
		if !v.Fresh {
			continue
		}
		var dr domain.Downloader
		if err := database.Get(ctx, db, "downloaders", did, &dr); err != nil {
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			continue
		}
		have := jobIndex(snapshotJobs(v, dr.Categories()))
		if m.clientLooksEmpty(did, len(v.Items), len(list)) {
			continue
		}
		m.checkFinished(ctx, e, dr, list, have, v, true, v.ReadAt)
	}
}
