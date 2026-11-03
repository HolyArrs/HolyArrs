package downloader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// ErrNotActive means the Download is not one HolyArrs is still following.
var ErrNotActive = errors.New("this download is not active any more")

// dangerousExt are file types that run as a program. A Download holding one
// is failed (setting fail_dangerous), also when it hides behind a second
// extension such as movie.mkv.exe, behind trailing dots or spaces (Windows
// drops them), or behind a character that turns the name around on screen.
var dangerousExt = map[string]bool{".exe": true, ".scr": true, ".bat": true, ".cmd": true, ".com": true, ".pif": true, ".vbs": true,
	".js": true, ".jar": true, ".msi": true, ".ps1": true, ".lnk": true, ".apk": true, ".hta": true, ".wsf": true, ".wsh": true,
	".vbe": true, ".jse": true, ".msc": true, ".cpl": true, ".scf": true, ".reg": true, ".dll": true, ".sh": true, ".appimage": true}

func dangerous(files []File) []string {
	var bad []string
	for _, f := range files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		ext := strings.ToLower(path.Ext(strings.TrimRight(name, ". ")))
		if dangerousExt[ext] || strings.ContainsAny(name, "‭‮⁦⁧⁨⁩") {
			bad = append(bad, name)
		}
	}
	return bad
}

var archiveExt = map[string]bool{".zip": true, ".rar": true, ".7z": true, ".gz": true, ".tar": true, ".zipx": true, ".cab": true, ".arj": true}

// passwordNote is a "password" note for a locked archive: it is there to
// lure a person into running or opening something, so it always fails.
func passwordNote(name string) bool {
	low := strings.ToLower(name)
	ext := path.Ext(low)
	return (ext == ".txt" || ext == ".html" || ext == ".htm") && strings.Contains(low, "password")
}

// shortcut is a web shortcut ("Downloaded from X.url"). Ordinary public
// torrents carry them beside the media, so one fails a Download only when
// no video or audio sits beside it; otherwise it is only noted.
func shortcut(name string) bool {
	ext := path.Ext(strings.ToLower(name))
	return ext == ".url" || ext == ".webloc"
}

// hasMedia says whether a file list holds believable media: a video or a
// track of a kind the importer places, at least the judge's least size
// (judge.MinVideo, judge.MinTrack), or a disc image or disc part of at
// least the video floor. A tiny or empty "media" file beside archives or
// shortcuts is part of the bait, not media. Programs fail on their own.
func hasMedia(files []File) bool {
	for _, f := range files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		ext := path.Ext(strings.TrimRight(name, ". "))
		switch {
		case importer.IsAudio(ext) && f.Size >= judge.MinTrack:
			return true
		case (importer.IsVideo(ext) || importer.IsMedia(ext) && !importer.IsAudio(ext)) && f.Size >= judge.MinVideo:
			return true
		}
	}
	return false
}

// shortcuts lists the web shortcuts in a file list.
func shortcuts(files []File) []string {
	var out []string
	for _, f := range files {
		if name := path.Base(strings.ReplaceAll(f.Name, `\`, "/")); shortcut(name) {
			out = append(out, name)
		}
	}
	return out
}

// fakeFiles lists a Download's files that make it a fake (4.3.5, setting
// fail_dangerous): programs (dangerous), password notes, web shortcuts
// with no media beside them, and, for a torrent, archives with no video or
// audio beside them (a locked or program-filled archive in place of the
// media). A RAR set big enough to hold a video is not one: the importer
// unpacks it (#54) and refuses a locked or program-filled one itself.
func fakeFiles(files []File, protocol string) []string {
	bad := dangerous(files)
	media := hasMedia(files) || rarSetSize(files) >= judge.MinVideo
	var archives []string
	for _, f := range files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		ext := strings.ToLower(path.Ext(strings.TrimRight(name, ". ")))
		switch {
		case passwordNote(name):
			bad = append(bad, name)
		case shortcut(name):
			if !media {
				bad = append(bad, name)
			}
		case archiveExt[ext] || isRarPart(ext):
			archives = append(archives, name)
		}
	}
	if protocol == "torrent" && !media && len(archives) > 0 {
		bad = append(bad, archives...)
	}
	return bad
}

// rarSetSize adds up the RAR volumes in a file list (.rar, .partN.rar, .r00 ...).
func rarSetSize(files []File) int64 {
	var n int64
	for _, f := range files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		if ext := strings.ToLower(path.Ext(strings.TrimRight(name, ". "))); ext == ".rar" || isRarPart(ext) {
			n += f.Size
		}
	}
	return n
}

// isRarPart is a split archive part: .r00, .r01, ...
func isRarPart(ext string) bool {
	return len(ext) == 4 && ext[1] == 'r' && ext[2] >= '0' && ext[2] <= '9' && ext[3] >= '0' && ext[3] <= '9'
}

// fakeReason says in plain words why a Download's files are refused.
func fakeReason(files []File, protocol string) string {
	if d := dangerous(files); len(d) > 0 {
		return "dangerous files: " + strings.Join(d, ", ")
	}
	return "fake files instead of the media: " + strings.Join(fakeFiles(files, protocol), ", ")
}

// MarkFailed is the person's "Mark failed": the active Download fails with
// their name as the reason, then the failure path runs.
func (m *Manager) MarkFailed(ctx context.Context, downloadID int64, by domain.Actor, who string) error {
	var d domain.Download
	if err := store.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	if !isActive(d.State) {
		return ErrNotActive
	}
	var dr domain.Downloader
	if err := store.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return err
	}
	next := d
	next.State = "failed"
	reason := "marked failed"
	if who != "" {
		reason += " by " + who
	}
	next.StateReason = &reason
	if !m.failed(ctx, dr, d, next, m.settings(ctx), by) {
		return ErrNotActive
	}
	return nil
}

// failed is the failure path (contract section 6): the Download fails or
// vanishes, its Release is turned down for its Slots, each Slot it was
// filling moves to failed, the Download is removed from the Downloader
// (setting remove_failed; not when it vanished) and each Slot is searched
// again. A vanished Download with replace_vanished off sends its Slots to
// Needs a decision instead. It reports whether it changed anything.
func (m *Manager) failed(ctx context.Context, dr domain.Downloader, old, next domain.Download, st settings, by domain.Actor) bool {
	reason := "failed"
	if next.StateReason != nil {
		reason = *next.StateReason
	}
	next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = 0, 0, 0, 0, nil
	replace := next.State == "failed" || st.replaceVanished
	var moved []int64
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var cur string
		if err := tx.QueryRow("SELECT state FROM downloads WHERE id = ?", old.ID).Scan(&cur); err != nil {
			return err
		}
		if !isActive(cur) {
			return ErrNotActive // another path got there first
		}
		if err := store.Update(tx, "downloads", &next, by); err != nil {
			return err
		}
		if replace && next.ReleaseID != nil {
			if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL,
				verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?), updated_at = ? WHERE id = ?`,
				"the download failed: "+reason, store.FormatTime(m.now()), *next.ReleaseID); err != nil {
				return err
			}
		}
		for _, sid := range next.SlotIDs {
			var state string
			var active sql.NullInt64
			if err := tx.QueryRow("SELECT state, active_download_id FROM slots WHERE id = ?", sid).Scan(&state, &active); err != nil {
				continue
			}
			if !active.Valid || active.Int64 != old.ID || (state != string(domain.SlotDownloading) && state != string(domain.SlotStalled)) {
				continue
			}
			if err := store.MoveSlotTx(tx, sid, domain.SlotFailed, reason, by); err != nil {
				return err
			}
			if !replace {
				if err := store.MoveSlotTx(tx, sid, domain.SlotNeedsADecision, reason+"; not replaced (setting)", by); err != nil {
					return err
				}
			}
			moved = append(moved, sid)
		}
		return nil
	})
	if err != nil {
		return false
	}
	m.publish("download", next)
	kind := "download_failed"
	if next.State == "vanished" {
		kind = "download_vanished"
	}
	m.event(ctx, &next, kind, "Download failed: "+reason)
	if next.State == "failed" && st.removeFailed {
		if e, err := m.engine(ctx, dr); err == nil {
			if err := e.Remove(ctx, next.ExternalID, true); err != nil && !errors.Is(err, ErrNotFound) {
				m.event(ctx, &next, "download_not_removed", fmt.Sprintf("Could not remove the failed download from %s: %v", dr.Name, firstLine(err.Error())))
			}
		}
	}
	for _, sid := range moved {
		state := domain.SlotFailed
		if !replace {
			state = domain.SlotNeedsADecision
		}
		m.publish("slot", map[string]any{"id": sid, "state": state, "reason": reason})
		if replace && m.OnFailed != nil {
			m.OnFailed(ctx, sid)
		}
	}
	return true
}

// Limits on reading a finished folder by hand.
const (
	walkMaxFiles = 10000
	walkMaxDepth = 6
)

// readFinished lists the files of a finished Download whose Downloader
// cannot list them (SABnzbd): it reads the finished folder itself, after
// the path map, skipping links. Names are relative to the returned save
// path. It reads only under a path-map folder of ours or the save root
// (saveRoot, as we see it), at most walkMaxFiles files and walkMaxDepth
// folders deep. When the folder cannot be read here, a content path that
// names a file is taken as that one file.
func readFinished(j Job, pm []domain.PathMap, saveRoot string) (string, []File) {
	content := j.ContentPath
	if content == "" {
		return j.SavePath, nil
	}
	save, base := parentDir(content), path.Base(strings.ReplaceAll(content, `\`, "/"))
	local := filepath.Clean(mapLocal(content, pm))
	roots := []string{saveRoot}
	for _, m := range pm {
		roots = append(roots, m.Ours)
	}
	fi, err := os.Lstat(local)
	if !underAny(local, roots) {
		err = fs.ErrPermission
	}
	switch {
	case err == nil && fi.Mode().IsRegular():
		return save, []File{{Name: base, Size: fi.Size(), Progress: 1}}
	case err == nil && fi.IsDir():
		var out []File
		_ = filepath.WalkDir(local, func(p string, d fs.DirEntry, err error) error {
			if len(out) >= walkMaxFiles {
				return fs.SkipAll
			}
			if err != nil || d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if rel, rerr := filepath.Rel(local, p); rerr == nil && rel != "." && strings.Count(filepath.ToSlash(rel), "/")+1 > walkMaxDepth {
					return fs.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(local, p)
			if err != nil {
				return nil
			}
			out = append(out, File{Name: base + "/" + filepath.ToSlash(rel), Size: info.Size(), Progress: 1})
			return nil
		})
		return save, out
	}
	if ext := path.Ext(base); ext != "" && len(ext) <= 5 {
		return save, []File{{Name: base, Size: j.Size, Progress: 1}}
	}
	return j.SavePath, nil
}

// underAny says whether p (clean) is one of roots or inside one.
func underAny(p string, roots []string) bool {
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		r = filepath.Clean(r)
		if p == r {
			return true
		}
		if rel, err := filepath.Rel(r, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

// mapLocal turns a Downloader's path into ours with the Downloader's path map.
func mapLocal(p string, maps []domain.PathMap) string {
	for _, m := range maps {
		theirs, ours := strings.TrimRight(m.Theirs, `/\`), strings.TrimRight(m.Ours, `/\`)
		if theirs == "" {
			continue
		}
		if p == theirs {
			return ours
		}
		if rest, ok := strings.CutPrefix(p, theirs); ok && (rest[0] == '/' || rest[0] == '\\') {
			return ours + rest
		}
	}
	return p
}

// Discard removes a Download from its Downloader with its files after its
// Import was rejected (lying files), when remove_failed is on. The
// Download row stays as history.
func (m *Manager) Discard(ctx context.Context, downloadID int64) error {
	if !m.settings(ctx).removeFailed {
		return nil
	}
	var d domain.Download
	if err := store.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	var dr domain.Downloader
	if err := store.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return err
	}
	e, err := m.engine(ctx, dr)
	if err != nil {
		return err
	}
	if err := e.Remove(ctx, d.ExternalID, true); err != nil && !errors.Is(err, ErrNotFound) {
		m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Could not remove the rejected download from %s: %v", dr.Name, firstLine(err.Error())))
		return err
	}
	m.event(ctx, &d, "download_removed", "Removed the rejected download from "+dr.Name)
	return nil
}

// RemoveChoice is what a person picks when removing a Download by hand
// (Radarr's queue remove: out of the Downloader, turn the Release down,
// search again). SearchAgain counts only with TurnDown, as in Radarr.
type RemoveChoice struct {
	DeleteFiles bool `json:"delete_files"`
	TurnDown    bool `json:"turn_down"`
	SearchAgain bool `json:"search_again"`
}

// Remove takes an active Download away by hand: it is removed from its
// Downloader (with or without its files), the row becomes cleaned with
// "removed by <name>", the Release is turned down when asked, and each Slot
// it was filling is searched again (turned down and search again) or goes
// back to wanted (better_wanted with a copy in place).
func (m *Manager) Remove(ctx context.Context, downloadID int64, c RemoveChoice, by domain.Actor, who string) error {
	var d domain.Download
	if err := store.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	if !isActive(d.State) {
		return ErrNotActive
	}
	var dr domain.Downloader
	if err := store.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return err
	}
	reason := "removed"
	if who != "" {
		reason += " by " + who
	}
	next := d
	next.State, next.StateReason, next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = "cleaned", &reason, 0, 0, 0, 0, nil
	search := c.TurnDown && c.SearchAgain
	var moved []int64
	rest := map[int64]domain.SlotState{}
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var cur string
		if err := tx.QueryRow("SELECT state FROM downloads WHERE id = ?", d.ID).Scan(&cur); err != nil {
			return err
		}
		if !isActive(cur) {
			return ErrNotActive
		}
		if err := store.Update(tx, "downloads", &next, by); err != nil {
			return err
		}
		// Recorded first, so a crash or a Downloader that is down never
		// leaves the download behind: the sync tries again until it is gone.
		if _, err := tx.Exec("UPDATE downloads SET removal_pending = ? WHERE id = ?", c.DeleteFiles, d.ID); err != nil {
			return err
		}
		if c.TurnDown && next.ReleaseID != nil {
			if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL,
				verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?), updated_at = ? WHERE id = ?`,
				"the download was "+reason, store.FormatTime(m.now()), *next.ReleaseID); err != nil {
				return err
			}
		}
		for _, sid := range next.SlotIDs {
			var state string
			var active, file sql.NullInt64
			if err := tx.QueryRow("SELECT state, active_download_id, file_id FROM slots WHERE id = ?", sid).Scan(&state, &active, &file); err != nil {
				continue
			}
			if !active.Valid || active.Int64 != d.ID || (state != string(domain.SlotDownloading) && state != string(domain.SlotStalled)) {
				continue
			}
			if err := store.MoveSlotTx(tx, sid, domain.SlotFailed, reason, by); err != nil {
				return err
			}
			if !search {
				to := domain.SlotWanted
				if file.Valid {
					to = domain.SlotBetterWanted
				}
				if err := store.MoveSlotTx(tx, sid, to, reason, by); err != nil {
					return err
				}
				rest[sid] = to
			}
			moved = append(moved, sid)
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.publish("download", next)
	m.event(ctx, &next, "download_removed", "Download "+reason)
	if e, err := m.engine(ctx, dr); err == nil {
		if err := e.Remove(ctx, next.ExternalID, c.DeleteFiles); err != nil && !errors.Is(err, ErrNotFound) {
			m.event(ctx, &next, "download_not_removed", fmt.Sprintf("Could not remove the download from %s: %v; HolyArrs tries again on the next sync", dr.Name, firstLine(err.Error())))
		} else {
			m.removalDone(ctx, d.ID)
		}
	}
	for _, sid := range moved {
		if search {
			m.publish("slot", map[string]any{"id": sid, "state": domain.SlotFailed, "reason": reason})
			if m.OnFailed != nil {
				m.OnFailed(ctx, sid)
			}
			continue
		}
		m.publish("slot", map[string]any{"id": sid, "state": rest[sid], "reason": reason})
	}
	return nil
}

// removalDone clears a recorded removal once the Downloader confirmed it.
func (m *Manager) removalDone(ctx context.Context, id int64) {
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET removal_pending = NULL WHERE id = ?", id)
		return err
	})
}

// retryRemovals asks each Downloader again to remove the downloads a Remove
// could not reach (the Downloader was down, or HolyArrs stopped first).
func (m *Manager) retryRemovals(ctx context.Context) {
	type pending struct {
		id, downloader int64
		external       string
		deleteFiles    bool
	}
	rows, err := m.Store.Read().QueryContext(ctx, "SELECT id, downloader_id, external_id, removal_pending FROM downloads WHERE removal_pending IS NOT NULL ORDER BY id")
	if err != nil {
		return
	}
	var list []pending
	for rows.Next() {
		var p pending
		if rows.Scan(&p.id, &p.downloader, &p.external, &p.deleteFiles) == nil {
			list = append(list, p)
		}
	}
	_ = rows.Close()
	for _, p := range list {
		var dr domain.Downloader
		if err := store.Get(ctx, m.Store.Read(), "downloaders", p.downloader, &dr); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				m.removalDone(ctx, p.id)
			}
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			continue
		}
		if err := e.Remove(ctx, p.external, p.deleteFiles); err == nil || errors.Is(err, ErrNotFound) {
			m.removalDone(ctx, p.id)
		}
	}
}
