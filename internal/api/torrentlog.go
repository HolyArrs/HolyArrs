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
}

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
		host := hostOnlyWord(t.URL)
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

// hostOnlyWord is a tracker address's host name alone.
func hostOnlyWord(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?@"); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}

// noteJobs compares a Downloader's list with the last one and records what
// changed. The first sighting after a start only remembers.
func (a *API) noteJobs(ctx context.Context, downloaderID int64, jobs []downloader.Job) {
	now := time.Now().UTC()
	type rec struct {
		hash  string
		lines []DownloadLogLine
	}
	var recs []rec
	a.tlog.mu.Lock()
	if a.tlog.seen == nil {
		a.tlog.seen = map[clientKey]torrentSnap{}
	}
	for _, j := range jobs {
		if j.ID == "" {
			continue
		}
		k := clientKey{downloaderID, j.ID}
		cur := snapOf(j)
		prev, ok := a.tlog.seen[k]
		a.tlog.seen[k] = cur
		if !ok {
			continue
		}
		if ls := stateLines(prev, cur); len(ls) > 0 {
			recs = append(recs, rec{j.ID, ls})
		}
	}
	a.tlog.mu.Unlock()
	for _, r := range recs {
		a.writeTorrentLog(ctx, downloaderID, r.hash, now, r.lines)
	}
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

// lastActivity is the last piece-moved time the list refresh saw.
func (a *API) lastActivity(downloaderID int64, hash string) int64 {
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
