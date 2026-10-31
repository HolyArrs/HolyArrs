package api

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// The torrent log: what happened to a torrent, for the panel's Log tab.
// Three sources are merged: lines worked out from the client's own
// timestamps, state changes HolyArrs sees in the list refresh, and tracker
// message changes. Changes are compared in memory and written only when
// something changed; nothing here asks the client anything extra.

// torrentLogCap is how many recorded lines each torrent keeps.
const torrentLogCap = 200

// torrentSnap is what the last list refresh saw of one torrent.
type torrentSnap struct {
	detail, savePath, trackerProblem string
	progress                         float64
	ratio                            float64
	seedingTime, lastActivity        int64
	limits                           *downloader.ShareLimits
}

// torrentLog keeps the last seen snapshot and tracker messages per torrent.
type torrentLog struct {
	mu       sync.Mutex
	seen     map[clientKey]torrentSnap
	trackers map[clientKey]map[string]string
	// seenDay is the day each torrent's torrent_seen row was last written.
	seenDay   map[clientKey]string
	lastSweep time.Time
}

// torrentForget is how long a torrent may go unseen before its log and link
// rows are swept.
const torrentForget = 30 * 24 * time.Hour

func snapOf(j downloader.Job) torrentSnap {
	return torrentSnap{detail: j.Detail, savePath: j.SavePath, trackerProblem: j.TrackerProblem, progress: j.Progress,
		ratio: j.Ratio, seedingTime: j.SeedingTime, lastActivity: j.LastActivity, limits: j.Limits}
}

func seedingWord(s string) bool {
	switch s {
	case "uploading", "stalledUP", "forcedUP", "queuedUP":
		return true
	}
	return false
}

func stoppedWord(s string) bool {
	return strings.HasPrefix(s, "paused") || strings.HasPrefix(s, "stopped")
}

// ruleReached says a torrent's own seeding rule is met.
func ruleReached(s torrentSnap) bool {
	l := s.limits
	if l == nil {
		return false
	}
	return l.Ratio >= 0 && s.ratio >= l.Ratio || l.SeedingMinutes >= 0 && s.seedingTime >= l.SeedingMinutes*60
}

// stateLines are the log lines for one torrent going from prev to cur.
func stateLines(prev, cur torrentSnap) []DownloadLogLine {
	var out []DownloadLogLine
	add := func(kind, summary string) { out = append(out, DownloadLogLine{Kind: kind, Summary: summary}) }
	if prev.progress < 1 && cur.progress >= 1 {
		add("completed", "Completed")
	}
	if prev.detail != cur.detail {
		switch {
		case cur.detail == "error":
			add("errored", "Errored")
		case cur.detail == "missingFiles":
			add("missing", "Missing files")
		case cur.detail == "moving":
			add("moving", "Moving")
		case strings.HasPrefix(cur.detail, "checking"):
			add("checking", "Checking")
		case stoppedWord(cur.detail) && !stoppedWord(prev.detail):
			if seedingWord(prev.detail) && ruleReached(cur) {
				add("rule", "Seeding rule reached")
			} else {
				add("paused", "Paused")
			}
		case cur.detail == "stalledDL" && prev.detail != "stalledDL":
			add("stalled", "Stalled")
		case stoppedWord(prev.detail) && !stoppedWord(cur.detail):
			add("resumed", "Resumed")
		}
	}
	if prev.savePath != "" && cur.savePath != "" && prev.savePath != cur.savePath {
		add("moved", "Moved to "+cur.savePath)
	}
	if prev.trackerProblem != cur.trackerProblem {
		switch cur.trackerProblem {
		case "error":
			add("tracker", "Tracker error")
		case "warning":
			add("tracker", "Tracker warning")
		case "announce":
			add("tracker", "No tracker answered")
		case "":
			add("tracker", "Trackers working again")
		}
	}
	return out
}

// trackerLines are the lines for tracker messages that changed. The first
// sighting of a tracker records nothing; an empty new message records
// nothing either. Hosts only: addresses in messages are cut to their host.
func trackerLines(prev map[string]string, cur []downloader.Tracker) ([]DownloadLogLine, map[string]string) {
	next := map[string]string{}
	var out []DownloadLogLine
	for _, t := range cur {
		if t.Fixed {
			continue
		}
		host := downloader.HostOnly(t.URL)
		msg := strings.TrimSpace(downloader.RedactURLs(t.Message))
		if msg == "" {
			msg = t.Status
		}
		next[host] = msg
		if was, ok := prev[host]; prev != nil && ok && was != msg && msg != "" {
			out = append(out, DownloadLogLine{Kind: "tracker", Summary: host + ": " + msg})
		}
	}
	return out, next
}

// noteJobs compares a Downloader's list with the last one and records what
// changed. The first sighting after a start only remembers. jobs is the
// whole list: torrents no longer in it are dropped from memory.
func (a *API) noteJobs(ctx context.Context, downloaderID int64, jobs []downloader.Job) {
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	type rec struct {
		hash  string
		lines []DownloadLogLine
	}
	var recs []rec
	var fresh []string
	a.tlog.mu.Lock()
	if a.tlog.seen == nil {
		a.tlog.seen = map[clientKey]torrentSnap{}
	}
	if a.tlog.seenDay == nil {
		a.tlog.seenDay = map[clientKey]string{}
	}
	here := map[string]bool{}
	for _, j := range jobs {
		if j.ID == "" {
			continue
		}
		hash := strings.ToLower(j.ID)
		here[hash] = true
		k := clientKey{downloaderID, hash}
		if a.tlog.seenDay[k] != day {
			a.tlog.seenDay[k] = day
			fresh = append(fresh, hash)
		}
		cur := snapOf(j)
		prev, ok := a.tlog.seen[k]
		a.tlog.seen[k] = cur
		if !ok {
			continue
		}
		if ls := stateLines(prev, cur); len(ls) > 0 {
			recs = append(recs, rec{hash, ls})
		}
	}
	for k := range a.tlog.seen {
		if k.downloader == downloaderID && !here[k.hash] {
			delete(a.tlog.seen, k)
		}
	}
	for k := range a.tlog.trackers {
		if k.downloader == downloaderID && !here[k.hash] {
			delete(a.tlog.trackers, k)
		}
	}
	for k := range a.tlog.seenDay {
		if k.downloader == downloaderID && !here[k.hash] {
			delete(a.tlog.seenDay, k)
		}
	}
	a.tlog.mu.Unlock()
	for _, r := range recs {
		a.writeTorrentLog(ctx, downloaderID, r.hash, now, r.lines)
	}
	if len(fresh) > 0 {
		at := now.Format(store.TimeLayout)
		_ = a.s.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
			for _, h := range fresh {
				if _, err := tx.Exec(`INSERT INTO torrent_seen (downloader_id, hash, seen_at) VALUES (?, ?, ?)
					ON CONFLICT (downloader_id, hash) DO UPDATE SET seen_at = excluded.seen_at`, downloaderID, h, at); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

// maybeSweep runs the daily sweep. Callers run it only after every
// Downloader's list was read and noted; skip names the Downloaders whose
// list read failed, so their rows are not stale merely because they were
// not read.
func (a *API) maybeSweep(ctx context.Context, skip []int64) {
	now := time.Now().UTC()
	a.tlog.mu.Lock()
	due := now.Sub(a.tlog.lastSweep) >= 24*time.Hour
	if due {
		a.tlog.lastSweep = now
	}
	a.tlog.mu.Unlock()
	if due {
		a.sweepTorrentRows(ctx, now, skip...)
	}
}

// sweepTorrentRows drops log and link rows of torrents not seen for
// torrentForget, leaving the skipped Downloaders alone. Rows from before
// torrent_seen existed go by their own age. Rows keyed by a hash with
// capitals (from before hashes were stored lower-case) go too.
func (a *API) sweepTorrentRows(ctx context.Context, now time.Time, skip ...int64) {
	cut := now.Add(-torrentForget).Format(store.TimeLayout)
	not, skipArgs := "", []any{}
	if len(skip) > 0 {
		not = " AND downloader_id NOT IN (?" + strings.Repeat(",?", len(skip)-1) + ")"
		for _, id := range skip {
			skipArgs = append(skipArgs, id)
		}
	}
	_ = a.s.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		stale := `SELECT downloader_id, hash FROM torrent_seen WHERE seen_at < ?`
		if _, err := tx.Exec(`DELETE FROM torrent_log WHERE ((downloader_id, hash) IN (`+stale+`)
			OR (at < ? AND (downloader_id, hash) NOT IN (SELECT downloader_id, hash FROM torrent_seen)))`+not, append([]any{cut, cut}, skipArgs...)...); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM client_links WHERE ((downloader_id, hash) IN (`+stale+`)
			OR (matched_at < ? AND (downloader_id, hash) NOT IN (SELECT downloader_id, hash FROM torrent_seen)))`+not, append([]any{cut, cut}, skipArgs...)...); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM torrent_seen WHERE seen_at < ?`+not, append([]any{cut}, skipArgs...)...); err != nil {
			return err
		}
		for _, t := range []string{"torrent_log", "client_links", "torrent_seen"} {
			if _, err := tx.Exec("DELETE FROM " + t + " WHERE hash <> lower(hash)"); err != nil {
				return err
			}
		}
		return nil
	})
}

// forgetTorrents drops what HolyArrs keeps about torrents removed through
// it: memory, log, link and seen rows. No hashes means the whole Downloader.
// forgetChunk caps the hashes in one DELETE ... IN (...).
const forgetChunk = 500

func (a *API) forgetTorrents(ctx context.Context, downloaderID int64, hashes []string) {
	all := len(hashes) == 0
	gone := map[string]bool{}
	for _, h := range hashes {
		gone[strings.ToLower(h)] = true
	}
	a.tlog.mu.Lock()
	for k := range a.tlog.seen {
		if k.downloader == downloaderID && (all || gone[k.hash]) {
			delete(a.tlog.seen, k)
		}
	}
	for k := range a.tlog.trackers {
		if k.downloader == downloaderID && (all || gone[k.hash]) {
			delete(a.tlog.trackers, k)
		}
	}
	for k := range a.tlog.seenDay {
		if k.downloader == downloaderID && (all || gone[k.hash]) {
			delete(a.tlog.seenDay, k)
		}
	}
	a.tlog.mu.Unlock()
	list := make([]string, 0, len(gone))
	for h := range gone {
		list = append(list, h)
	}
	_ = a.s.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		for _, t := range []string{"torrent_log", "client_links", "torrent_seen"} {
			if all {
				if _, err := tx.Exec("DELETE FROM "+t+" WHERE downloader_id = ?", downloaderID); err != nil {
					return err
				}
				continue
			}
			for i := 0; i < len(list); i += forgetChunk {
				part := list[i:min(i+forgetChunk, len(list))]
				args := make([]any, 0, len(part)+1)
				args = append(args, downloaderID)
				for _, h := range part {
					args = append(args, h)
				}
				q := "DELETE FROM " + t + " WHERE downloader_id = ? AND hash IN (?" + strings.Repeat(",?", len(part)-1) + ")"
				if _, err := tx.Exec(q, args...); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// noteTrackers records tracker message changes from a swarm read.
func (a *API) noteTrackers(ctx context.Context, downloaderID int64, hash string, ts []downloader.Tracker) {
	hash = strings.ToLower(hash)
	k := clientKey{downloaderID, hash}
	a.tlog.mu.Lock()
	if a.tlog.trackers == nil {
		a.tlog.trackers = map[clientKey]map[string]string{}
	}
	lines, next := trackerLines(a.tlog.trackers[k], ts)
	a.tlog.trackers[k] = next
	a.tlog.mu.Unlock()
	if len(lines) > 0 {
		a.writeTorrentLog(ctx, downloaderID, hash, time.Now().UTC(), lines)
	}
}

// lastActivity is the last piece-moved time the list refresh saw; with
// the background reader on, the reader's snapshot (the list cache stays
// empty then).
func (a *API) lastActivity(downloaderID int64, hash string) int64 {
	if a.syncOn() {
		return a.s.DLSync.Store.Current(downloaderID).Items[strings.ToLower(hash)].LastActivity
	}
	a.tlog.mu.Lock()
	defer a.tlog.mu.Unlock()
	return a.tlog.seen[clientKey{downloaderID, strings.ToLower(hash)}].lastActivity
}

// writeTorrentLog stores lines and keeps the newest torrentLogCap.
func (a *API) writeTorrentLog(ctx context.Context, downloaderID int64, hash string, at time.Time, lines []DownloadLogLine) {
	_ = a.s.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		for _, l := range lines {
			if _, err := tx.Exec("INSERT INTO torrent_log (downloader_id, hash, at, kind, summary) VALUES (?, ?, ?, ?, ?)",
				downloaderID, hash, at.Format(store.TimeLayout), l.Kind, l.Summary); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`DELETE FROM torrent_log WHERE downloader_id = ? AND hash = ? AND id NOT IN
			(SELECT id FROM torrent_log WHERE downloader_id = ? AND hash = ? ORDER BY at DESC, id DESC LIMIT ?)`,
			downloaderID, hash, downloaderID, hash, torrentLogCap)
		return err
	})
}

// recordedLines reads a torrent's recorded lines.
func (a *API) recordedLines(ctx context.Context, downloaderID int64, hash string) []DownloadLogLine {
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT at, kind, summary FROM torrent_log WHERE downloader_id = ? AND hash = ? ORDER BY at DESC, id DESC LIMIT ?",
		downloaderID, strings.ToLower(hash), torrentLogCap)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []DownloadLogLine
	for rows.Next() {
		var l DownloadLogLine
		var at string
		if rows.Scan(&at, &l.Kind, &l.Summary) == nil {
			l.At, _ = time.Parse(store.TimeLayout, at)
			out = append(out, l)
		}
	}
	return out
}

// derivedLines are the lines worked out from the client's own timestamps;
// only events with real values are shown.
func derivedLines(t *downloader.TorrentDetail, lastActivity int64, now time.Time) []DownloadLogLine {
	if t == nil {
		return nil
	}
	var out []DownloadLogLine
	if t.AddedAt != nil {
		out = append(out, DownloadLogLine{At: *t.AddedAt, Kind: "added", Summary: "Added"})
	}
	if t.DoneAt != nil {
		s := "Finished downloading"
		if t.AddedAt != nil && t.DoneAt.After(*t.AddedAt) {
			s += " in " + plainSpan(t.DoneAt.Sub(*t.AddedAt))
		}
		out = append(out, DownloadLogLine{At: *t.DoneAt, Kind: "finished", Summary: s})
	}
	if t.SeedingTime > 0 {
		at := now.Add(-time.Duration(t.SeedingTime) * time.Second).Truncate(time.Minute)
		if t.DoneAt != nil && at.Before(*t.DoneAt) {
			at = *t.DoneAt
		}
		out = append(out, DownloadLogLine{At: at, Kind: "seeding", Summary: fmt.Sprintf("Seeding since · %s uploaded · ratio %.2f", plainBytes(t.Uploaded), t.Ratio)})
	}
	if lastActivity > 0 {
		at := time.Unix(lastActivity, 0).UTC()
		if t.AddedAt == nil || at.After(t.AddedAt.Add(time.Minute)) {
			out = append(out, DownloadLogLine{At: at, Kind: "activity", Summary: "Last activity"})
		}
	}
	if t.LastSeen != nil {
		out = append(out, DownloadLogLine{At: *t.LastSeen, Kind: "seen", Summary: "Last seen complete"})
	}
	return out
}

// mergeLog puts all lines newest first.
func mergeLog(parts ...[]DownloadLogLine) []DownloadLogLine {
	out := []DownloadLogLine{}
	for _, p := range parts {
		out = append(out, p...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func plainSpan(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

func plainBytes(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB", "GB"} {
		if f < 1024 {
			if u == "B" {
				return fmt.Sprintf("%d B", n)
			}
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1f TB", f)
}

// torrentTimeline merges a torrent's derived, recorded and HolyArrs lines,
// newest first, and fills the detail's last activity from the list.
func (a *API) torrentTimeline(ctx context.Context, downloaderID int64, hash string, t *downloader.TorrentDetail, own []DownloadLogLine) []DownloadLogLine {
	la := a.lastActivity(downloaderID, hash)
	if t != nil && la > 0 {
		at := time.Unix(la, 0).UTC()
		t.LastActivity = &at
	}
	return mergeLog(own, a.recordedLines(ctx, downloaderID, hash), derivedLines(t, la, time.Now().UTC()))
}

// seenRatio is the share ratio the last list read saw; nil when unseen.
func (a *API) seenRatio(downloaderID int64, hash string) *float64 {
	if hash == "" {
		return nil
	}
	a.tlog.mu.Lock()
	defer a.tlog.mu.Unlock()
	s, ok := a.tlog.seen[clientKey{downloaderID, strings.ToLower(hash)}]
	if !ok {
		return nil
	}
	r := s.ratio
	return &r
}
