package downloads

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// With setting downloads.sync = on the background reader (package dlsync)
// owns every client read: the Downloads view reads its snapshots and never
// calls a client, writes nothing and starts no matching. Matching runs in
// the reader's persisted queue, link-only (it never adds to the Library).

// syncOn says the background reader serves the Downloads view now.
func (a *Web) syncOn() bool {
	return a.DLSync != nil && a.DLSync.On()
}

// storeRows is clientRows from the reader's snapshots: no client call, no
// write, no matcher start.
func (a *Web) storeRows(ctx context.Context, names map[int64]domain.Downloader, shown map[clientKey]bool) ([]DownloadRow, []ClientHealth) {
	sy := a.DLSync
	health := []ClientHealth{}
	sab := sy.SABOutside(ctx)
	var ds []domain.Downloader
	for _, d := range names {
		if !d.Removed && (d.Protocol == "torrent" || sab && d.Protocol == "usenet") {
			ds = append(ds, d)
		}
	}
	sortDownloaders(ds)
	var rows []DownloadRow
	for _, d := range ds {
		snap := sy.Store.Current(d.ID)
		health = append(health, syncHealth(d, snap))
		out := make([]DownloadRow, 0, len(snap.Items))
		for _, j := range snap.Items {
			if j.ID == "" || shown[clientKey{d.ID, strings.ToLower(j.ID)}] {
				continue
			}
			out = append(out, clientRow(d, j))
		}
		sortClientRows(out)
		if len(out) > 0 {
			a.knownTitles(ctx, d.ID, out)
			a.linkedTitles(ctx, d.ID, out)
			a.rowOwners(ctx, out)
			rows = append(rows, out...)
		}
	}
	return rows, health
}

// syncHealth is one client's state as the background reader sees it.
func syncHealth(d domain.Downloader, snap Snapshot) ClientHealth {
	h := ClientHealth{ID: d.ID, Name: d.Name, Health: "ok", SeenAt: snap.ReadAt}
	switch {
	case snap.Fallback:
		h.Health, h.Stale = "degraded", !snap.Fresh
	case snap.Fresh:
	case snap.Stale || snap.ReadAt.IsZero() && len(snap.Items) > 0:
		h.Health, h.Stale = "checking", true
	case snap.ReadAt.IsZero():
		h.Health = "waiting"
	case snap.Failed:
		h.Health, h.Stale = "offline", true
	default:
		h.Health, h.Stale = "slow", true
	}
	return h
}

// SyncTrouble lists the torrent clients whose background reads are
// degraded (the Manager reads them itself) or being checked again, for the
// health page. Empty while the reader is off.
func (a *Web) SyncTrouble(ctx context.Context) []ClientHealth {
	out := []ClientHealth{}
	if !a.syncOn() {
		return out
	}
	ds, err := database.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1")
	if err != nil {
		return out
	}
	sortDownloaders(ds)
	for _, d := range ds {
		if d.Removed || d.Protocol != "torrent" {
			continue
		}
		if h := syncHealth(d, a.DLSync.Store.Current(d.ID)); h.Health == "degraded" || h.Health == "checking" {
			out = append(out, h)
		}
	}
	return out
}

// storeRatio is the share ratio in the reader's snapshot; nil when unseen.
func (a *Web) storeRatio(downloaderID int64, hash string) *float64 {
	if hash == "" {
		return nil
	}
	j, ok := a.DLSync.Store.Current(downloaderID).Items[strings.ToLower(hash)]
	if !ok {
		return nil
	}
	r := j.Ratio
	return &r
}

func sortDownloaders(ds []domain.Downloader) {
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Priority != ds[j].Priority {
			return ds[i].Priority < ds[j].Priority
		}
		if ds[i].Name != ds[j].Name {
			return ds[i].Name < ds[j].Name
		}
		return ds[i].ID < ds[j].ID
	})
}

// sortClientRows orders one client's rows newest first, then by hash.
func sortClientRows(out []DownloadRow) {
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i].StartedAt, out[j].StartedAt
		switch {
		case ai == nil || aj == nil:
			if (ai == nil) != (aj == nil) {
				return aj == nil
			}
		case !ai.Equal(*aj):
			return ai.After(*aj)
		}
		return out[i].Hash < out[j].Hash
	})
}

// TorrentLines is the torrent log's line format for one item going from
// prev to cur, for the reader's persister.
func TorrentLines(prev, cur Job) []LogLine {
	ls := stateLines(snapOf(prev), snapOf(cur))
	out := make([]LogLine, len(ls))
	for i, l := range ls {
		out[i] = LogLine{Kind: l.Kind, Summary: l.Summary}
	}
	return out
}

// MatchLinkOnly is the reader's match lookup: it links an item to a title
// already in the Library when the name is a sure match, and otherwise
// leaves the choices in the Confirm match list. It adds a missing title
// only with downloads_auto_add on.
func (a *Web) MatchLinkOnly(ctx context.Context, downloaderID int64, key, name, category string) (string, int64, error) {
	db := a.Store.Read()
	key = strings.ToLower(key)
	var n int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM downloads WHERE downloader_id = ? AND lower(external_id) = ?", downloaderID, key).Scan(&n) == nil && n > 0 {
		return "own", 0, nil
	}
	var d domain.Downloader
	if database.Get(ctx, db, "downloaders", downloaderID, &d) == nil && otherApp(d, category) {
		return matchOtherApp, 0, nil
	}
	var st, how string
	var tid *int64
	if db.QueryRowContext(ctx, "SELECT state, title_id, COALESCE(how, '') FROM client_links WHERE downloader_id = ? AND hash = ?", downloaderID, key).Scan(&st, &tid, &how) == nil &&
		!(how == "suggested" && tid == nil && a.autoAddOn(ctx)) && !(st == LinkNotMedia && how == "name") {
		// Already decided (by a person or an earlier match): leave it. Only
		// the matcher's own calls are looked at again: a suggestion once
		// auto-add is on, and a not-media name (the name reading improves).
		if tid != nil {
			return st, *tid, nil
		}
		return st, 0, nil
	}
	w := matchWant{downloader: downloaderID, hash: key, name: name, category: category}
	g := guessOf(name, category)
	if g.kind == "" {
		return LinkNotMedia, 0, a.saveLink(ctx, w, LinkNotMedia, 0, nil, "", nil, "name")
	}
	cs, err := a.candidatesFor(ctx, g)
	if err != nil {
		return "", 0, err
	}
	i := pickSure(cs, g.year)
	if i < 0 && g.kind == "show" && g.year == 0 {
		n, i = 0, -1
		for j, c := range cs {
			if c.Score >= sureScore {
				n, i = n+1, j
			}
		}
		if n != 1 {
			i = -1
		}
	}
	if i < 0 {
		if cs == nil {
			cs = []Candidate{}
		}
		return LinkUnsure, 0, a.saveLink(ctx, w, LinkUnsure, 0, nil, "", cs, "unsure")
	}
	c := cs[i]
	titleID, err := a.libraryTitle(ctx, c)
	if err != nil {
		return "", 0, err
	}
	added := false
	if titleID == 0 && a.autoAddOn(ctx) {
		// downloads_auto_add: add it the normal way (default folder and
		// pick rules, no search; the files are already in the client).
		// ensureTitle reuses a copy added before a restart, so never twice.
		titleID, err = a.ensureTitle(ctx, c)
		if err != nil && !isNoFolder(err) {
			return "", 0, err
		}
		added = err == nil && titleID != 0
	}
	if titleID == 0 {
		// A sure match not in the Library: suggested, waiting for a person
		// to confirm (which adds it the normal way).
		return "suggested", 0, a.saveLink(ctx, w, LinkUnsure, 0, nil, "", cs, "suggested")
	}
	slots, covers := a.linkSlots(ctx, titleID, c.Kind, g)
	how = "auto"
	if added {
		how = "auto add"
	}
	if err := a.saveLink(ctx, w, LinkLinked, titleID, slots, covers, cs, how); err != nil {
		return "", 0, err
	}
	if added {
		a.writeTorrentLog(ctx, downloaderID, key, time.Now().UTC(), []DownloadLogLine{{Kind: "library", Summary: "Added to your Library"}})
	}
	return LinkLinked, titleID, nil
}

// matchOtherApp is the match outcome for a Usenet job in a category
// HolyArrs does not use: another app's job in a shared client, shown on
// Downloads but never matched.
const matchOtherApp = "other"

// otherApp reports whether category belongs to another app on d: d is a
// Usenet client with categories of its own and category is none of them.
// A client with no category, "" or "*" has every job as its own.
func otherApp(d domain.Downloader, category string) bool {
	if d.Protocol != "usenet" {
		return false
	}
	cats := d.Categories()
	for _, c := range cats {
		if c == "" || c == "*" || strings.EqualFold(c, category) {
			return false
		}
	}
	return len(cats) > 0
}

// settingAutoAdd is downloads_auto_add (group links), off by default.
const settingAutoAdd = "downloads_auto_add"

// autoAddOn reads downloads_auto_add.
func (a *Web) autoAddOn(ctx context.Context) bool {
	var v string
	_ = a.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'links' AND key = ?", settingAutoAdd).Scan(&v)
	return v == "true"
}

// libraryTitle finds a candidate already in the Library (0 when missing),
// reading only.
func (a *Web) libraryTitle(ctx context.Context, c Candidate) (int64, error) {
	db := a.Store.Read()
	kind, v, _ := strings.Cut(c.ID, ":")
	if c.Kind == "movie" {
		return library.TitleByTMDB(ctx, db, v)
	}
	return library.ShowByIDs(ctx, db, map[string]string{kind: v})
}

// Rematch makes every waiting or given-up item in the reader's match queue
// due now (Match now, with sync on).
func (a *Web) Rematch(ctx context.Context) int {
	var n int64
	// Not-media names go again too (a person's not-media pick stays), and
	// with auto-add on, the matcher's suggestions, to be added.
	// Other apps' jobs too, in case HolyArrs's categories changed.
	outs := "('', 'nomatch', 'not_media', 'other')"
	if a.autoAddOn(ctx) {
		outs = "('', 'nomatch', 'not_media', 'other', 'suggested')"
	}
	_ = a.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE dl_match_queue SET attempts = 0, next_try_at = ?, outcome = '', last_error = '' WHERE outcome IN "+outs,
			time.Now().UTC().Format(database.TimeLayout))
		if err == nil {
			n, _ = res.RowsAffected()
		}
		return err
	})
	return int(n)
}
