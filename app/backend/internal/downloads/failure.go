package downloads

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

const (
	// Caps for walking a finished download on disk, so a huge or deeply nested tree cannot stall a sync.
	walkMaxFiles = 10000
	walkMaxDepth = 6
)

// ErrNotActive means the download already ended, so there is nothing to fail or remove.
var ErrNotActive = errors.New("this download is not active any more")

// dangerousExt lists file types that run code when opened. A Release with one is treated as malware, not media.
var dangerousExt = map[string]bool{".exe": true, ".scr": true, ".bat": true, ".cmd": true, ".com": true, ".pif": true, ".vbs": true,
	".js": true, ".jar": true, ".msi": true, ".ps1": true, ".lnk": true, ".apk": true, ".hta": true, ".wsf": true, ".wsh": true,
	".vbe": true, ".jse": true, ".msc": true, ".cpl": true, ".scf": true, ".reg": true, ".dll": true, ".sh": true, ".appimage": true}

// archiveExt lists archive types. They are only suspect on a torrent with no media and nothing big enough to unpack.
var archiveExt = map[string]bool{".zip": true, ".rar": true, ".7z": true, ".gz": true, ".tar": true, ".zipx": true, ".cab": true, ".arj": true}

// RemoveChoice is what the user picked when removing a download.
type RemoveChoice struct {
	DeleteFiles bool `json:"delete_files"`
	TurnDown    bool `json:"turn_down"`
	SearchAgain bool `json:"search_again"`
}

// MarkFailed fails an active download by hand. who is shown in the reason.
func (m *Manager) MarkFailed(ctx context.Context, downloadID int64, by domain.Actor, who string) error {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	if !isActive(d.State) {
		return ErrNotActive
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
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

// Discard removes a rejected download and its files from the client, only when the remove-failed setting is on. A download the client no longer has counts as done.
func (m *Manager) Discard(ctx context.Context, downloadID int64) error {
	if !m.settings(ctx).removeFailed {
		return nil
	}
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
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

// Remove ends an active download at the user's request. The row and slots change first and the client call comes after, so a client that is down cannot block the user; a retry is queued through removal_pending.
// TurnDown also rejects the Release. With SearchAgain the slots stay failed and OnFailed looks for another Release; otherwise they go back to wanted.
func (m *Manager) Remove(ctx context.Context, downloadID int64, c RemoveChoice, by domain.Actor, who string) error {
	_, err := m.RemoveSaid(ctx, downloadID, c, by, who)
	return err
}

// RemoveSaid is Remove that also returns the Downloader's refusal in words, or "".
func (m *Manager) RemoveSaid(ctx context.Context, downloadID int64, c RemoveChoice, by domain.Actor, who string) (string, error) {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return "", err
	}
	// A finished download whose Downloader was removed may go too: nothing
	// else can end it. Only HolyArrs' own record changes then.
	dr, gone, err := m.downloaderGone(ctx, d.DownloaderID)
	if err != nil {
		return "", err
	}
	// A finished download on a Downloader HolyArrs may change goes too: the
	// person stops its seeding or clears it, and the Library copy stays.
	removable := func(state string) bool { return isActive(state) || finishedState(state) }
	if !removable(d.State) {
		return "", ErrNotActive
	}
	kept, later := "", false
	if finishedState(d.State) && !gone {
		if err := m.doneRemovable(ctx, dr, d, &c); err != nil {
			return "", err
		}
		if c.DeleteFiles {
			if kept, later = m.doneKeepWhy(ctx, dr, d); kept != "" {
				c.DeleteFiles = false
			}
		}
	}
	reason := "removed"
	if who != "" {
		reason += " by " + who
	}
	next := d
	next.State, next.StateReason, next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = "cleaned", &reason, 0, 0, 0, 0, nil
	search := c.TurnDown && c.SearchAgain
	var moved []int64
	rest := map[int64]mediafiles.SlotState{}
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		// Read the state again inside the write so an earlier sync that ended the download wins.
		var cur string
		if err := tx.QueryRow("SELECT state FROM downloads WHERE id = ?", d.ID).Scan(&cur); err != nil {
			return err
		}
		if !removable(cur) {
			return ErrNotActive
		}
		if err := database.Update(tx, "downloads", &next, by); err != nil {
			return err
		}
		if !gone {
			pending := 0
			if later {
				pending = removalCheckLater
			} else if c.DeleteFiles {
				pending = 1
			}
			if _, err := tx.Exec("UPDATE downloads SET removal_pending = ? WHERE id = ?", pending, d.ID); err != nil {
				return err
			}
		}
		if c.TurnDown && next.ReleaseID != nil {
			if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL,
				verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?), updated_at = ? WHERE id = ?`,
				"the download was "+reason, database.FormatTime(m.now()), *next.ReleaseID); err != nil {
				return err
			}
		}
		for _, sid := range next.SlotIDs {
			var state string
			var active, file sql.NullInt64
			if err := tx.QueryRow("SELECT state, active_download_id, file_id FROM slots WHERE id = ?", sid).Scan(&state, &active, &file); err != nil {
				continue
			}
			if !active.Valid || active.Int64 != d.ID || (state != string(mediafiles.SlotDownloading) && state != string(mediafiles.SlotStalled)) {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotFailed, reason, by); err != nil {
				return err
			}
			if !search {
				to := mediafiles.SlotWanted
				if file.Valid {
					to = mediafiles.SlotBetterWanted
				}
				if err := mediafiles.MoveSlotTx(tx, sid, to, reason, by); err != nil {
					return err
				}
				rest[sid] = to
			}
			moved = append(moved, sid)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	m.publish("download", next)
	said := "Download " + reason
	extra := map[string]any{"delete_files": c.DeleteFiles}
	if kept != "" {
		said += "; its files were kept: " + kept
		extra["kept"] = kept
	}
	note := ""
	if later {
		// Nothing is removed yet: History says so now and says what
		// happened to the files once the later try works.
		note = dr.Name + " is not answering right now; HolyArrs tries again on the next sync"
		m.event(ctx, &next, "download_not_removed", note)
	} else {
		m.eventWith(ctx, &next, "download_removed", said, extra)
	}
	if !gone && !later {
		note = m.removeFromClient(ctx, dr, d.ID, &next, c.DeleteFiles)
	}
	for _, sid := range moved {
		if search {
			m.publish("slot", map[string]any{"id": sid, "state": mediafiles.SlotFailed, "reason": reason})
			if m.OnFailed != nil {
				m.OnFailed(ctx, sid)
			}
			continue
		}
		m.publish("slot", map[string]any{"id": sid, "state": rest[sid], "reason": reason})
	}
	return note, nil
}

// removeFromClient asks the Downloader to drop a removed download; a refusal
// stays in removal_pending and is asked again on the next sync.
// It returns that refusal in words, or "".
func (m *Manager) removeFromClient(ctx context.Context, dr domain.Downloader, id int64, next *domain.Download, deleteFiles bool) string {
	e, err := m.engine(ctx, dr)
	if err == nil {
		err = e.Remove(ctx, next.ExternalID, deleteFiles)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		summary := fmt.Sprintf("Could not remove the download from %s: %v; HolyArrs tries again on the next sync", dr.Name, firstLine(err.Error()))
		if errors.Is(err, outbound.ErrSuppressed) {
			summary = dr.Name + " is not answering right now; HolyArrs tries again on the next sync"
		}
		m.event(ctx, next, "download_not_removed", summary)
		return summary
	}
	m.removalDone(ctx, id)
	return ""
}

// dangerous returns names of files that run code or hide their type. The bidi control characters reverse text, so a name can look like a harmless extension.
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

// passwordNote spots the text file fake Releases ship to ask for a password.
func passwordNote(name string) bool {
	low := strings.ToLower(name)
	ext := path.Ext(low)
	return (ext == ".txt" || ext == ".html" || ext == ".htm") && strings.Contains(low, "password")
}

// shortcut spots internet shortcuts, a common way to point at a scam page.
func shortcut(name string) bool {
	ext := path.Ext(strings.ToLower(name))
	return ext == ".url" || ext == ".webloc"
}

// hasMedia reports whether any file is real media of a plausible size. Tiny samples do not count.
func hasMedia(files []File) bool {
	for _, f := range files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		ext := path.Ext(strings.TrimRight(name, ". "))
		switch {
		case importer.IsAudio(ext) && f.Size >= mediafiles.MinTrack:
			return true
		case (importer.IsVideo(ext) || importer.IsMedia(ext) && !importer.IsAudio(ext)) && f.Size >= mediafiles.MinVideo:
			return true
		}
	}
	return false
}

// shortcuts returns the names of internet shortcut files.
func shortcuts(files []File) []string {
	var out []string
	for _, f := range files {
		if name := path.Base(strings.ReplaceAll(f.Name, `\`, "/")); shortcut(name) {
			out = append(out, name)
		}
	}
	return out
}

// fakeFiles returns the names that make a Release look fake. Shortcuts are fine next to real media. Archives only count on a torrent with no media and no archive set big enough to unpack, since Usenet unpacks them itself.
func fakeFiles(files []File, protocol string) []string {
	bad := dangerous(files)
	media := hasMedia(files)
	unpackable := archiveSetSize(files) >= mediafiles.MinVideo
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
	if protocol == "torrent" && !media && !unpackable && len(archives) > 0 {
		bad = append(bad, archives...)
	}
	return bad
}

// archiveSetSize adds up the size of all archive parts.
func archiveSetSize(files []File) int64 {
	var n int64
	for _, f := range files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		ext := strings.ToLower(path.Ext(strings.TrimRight(name, ". ")))
		if k, _ := importer.ArchiveKind(name); ext == ".rar" || isRarPart(ext) || k != "" {
			n += f.Size
		}
	}
	return n
}

// isRarPart matches old style rar parts such as .r00 to .r99.
func isRarPart(ext string) bool {
	return len(ext) == 4 && ext[1] == 'r' && ext[2] >= '0' && ext[2] <= '9' && ext[3] >= '0' && ext[3] <= '9'
}

// fakeReason words the failure reason for a fake or dangerous Release.
func fakeReason(files []File, protocol string) string {
	if d := dangerous(files); len(d) > 0 {
		return "dangerous files: " + strings.Join(d, ", ")
	}
	return "fake files instead of the media: " + strings.Join(fakeFiles(files, protocol), ", ")
}

// failed ends a download as failed or vanished and moves its slots. A failed download always replaces the Release; a vanished one only when the setting says so, else the slot waits for a user decision.
// It returns false when the download had already ended.
func (m *Manager) failed(ctx context.Context, dr domain.Downloader, old, next domain.Download, st settings, by domain.Actor) bool {
	reason := "failed"
	if next.StateReason != nil {
		reason = *next.StateReason
	}
	next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = 0, 0, 0, 0, nil
	// replace is false for a vanished download when the user chose not to replace those.
	replace := next.State == "failed" || st.replaceVanished
	var moved []int64
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var cur string
		if err := tx.QueryRow("SELECT state FROM downloads WHERE id = ?", old.ID).Scan(&cur); err != nil {
			return err
		}
		if !isActive(cur) {
			return ErrNotActive
		}
		if err := database.Update(tx, "downloads", &next, by); err != nil {
			return err
		}
		if replace && next.ReleaseID != nil {
			if _, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL,
				verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?), updated_at = ? WHERE id = ?`,
				"the download failed: "+reason, database.FormatTime(m.now()), *next.ReleaseID); err != nil {
				return err
			}
		}
		for _, sid := range next.SlotIDs {
			var state string
			var active sql.NullInt64
			if err := tx.QueryRow("SELECT state, active_download_id FROM slots WHERE id = ?", sid).Scan(&state, &active); err != nil {
				continue
			}
			if !active.Valid || active.Int64 != old.ID || (state != string(mediafiles.SlotDownloading) && state != string(mediafiles.SlotStalled)) {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotFailed, reason, by); err != nil {
				return err
			}
			if !replace {
				if err := mediafiles.MoveSlotTx(tx, sid, mediafiles.SlotNeedsADecision, reason+"; not replaced (setting)", by); err != nil {
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
			// Keep the files when the client itself reported the failure, so the user can look at them.
			deleteFiles := !strings.HasPrefix(reason, clientSaysPrefix)
			if err := e.Remove(ctx, next.ExternalID, deleteFiles); err != nil && !errors.Is(err, ErrNotFound) {
				m.event(ctx, &next, "download_not_removed", fmt.Sprintf("Could not remove the failed download from %s: %v", dr.Name, firstLine(err.Error())))
			}
		}
	}
	for _, sid := range moved {
		state := mediafiles.SlotFailed
		if !replace {
			state = mediafiles.SlotNeedsADecision
		}
		m.publish("slot", map[string]any{"id": sid, "state": state, "reason": reason})
		if replace && m.OnFailed != nil {
			m.OnFailed(ctx, sid)
		}
	}
	return true
}

// failDead fails a download the dead or crawling torrent rule caught. Like a
// stall, the Release is turned down and the Slots are searched again.
func (m *Manager) failDead(ctx context.Context, dr domain.Downloader, d domain.Download, reason string, st settings) {
	next := d
	next.State, next.StateReason = "failed", &reason
	m.failed(ctx, dr, d, next, st, domain.System)
}

// readFinished finds the final files of a finished job. It reads the disk through the path maps and never leaves the save root or the mapped roots, so a client cannot point HolyArrs at other files. If the path is not visible here, it falls back to the name and size the client gave.
func readFinished(j Job, pm []domain.PathMap, saveRoot string) (string, []File) {
	return readFinishedIn(context.Background(), j, pm, saveRoot)
}

// readFinishedIn is readFinished that stops walking a folder when ctx ends; what it read until then is not the whole list.
func readFinishedIn(ctx context.Context, j Job, pm []domain.PathMap, saveRoot string) (string, []File) {
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
	// Lstat so a symlink is not followed out of the roots.
	fi, err := os.Lstat(local)
	// Only inside the save root or a path pair the owner set up: a Downloader
	// never decides which folders HolyArrs reads.
	if !underAny(local, roots) {
		err = fs.ErrPermission
	}
	switch {
	case err == nil && fi.Mode().IsRegular():
		return save, []File{{Name: base, Size: fi.Size(), Progress: 1}}
	case err == nil && fi.IsDir():
		var out []File
		_ = filepath.WalkDir(local, func(p string, d fs.DirEntry, err error) error {
			if len(out) >= walkMaxFiles || ctx.Err() != nil {
				return fs.SkipAll
			}
			// Symlinks are skipped so the walk cannot leave the roots.
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
	// A short extension suggests a single file, not a folder.
	if ext := path.Ext(base); ext != "" && len(ext) <= 5 {
		return save, []File{{Name: base, Size: j.Size, Progress: 1}}
	}
	return j.SavePath, nil
}

// underAny reports whether p is inside any of the roots. Comparing relative paths avoids the prefix trap where /data2 would match /data.
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

// mapLocal turns the client's path into ours.
func mapLocal(p string, maps []domain.PathMap) string {
	out, _ := domain.MapPath(p, maps)
	return out
}

// removalDone clears the pending client removal.
func (m *Manager) removalDone(ctx context.Context, id int64) {
	_ = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET removal_pending = NULL WHERE id = ?", id)
		return err
	})
}

// retryRemovals repeats client removals that failed earlier. A missing client row or a download the client no longer has ends the retry.
func (m *Manager) retryRemovals(ctx context.Context) {
	type pending struct {
		id, downloader int64
		external       string
		pending        int64
	}
	rows, err := m.Store.Read().QueryContext(ctx, "SELECT id, downloader_id, external_id, removal_pending FROM downloads WHERE removal_pending IS NOT NULL ORDER BY id")
	if err != nil {
		return
	}
	var list []pending
	for rows.Next() {
		var p pending
		if rows.Scan(&p.id, &p.downloader, &p.external, &p.pending) == nil {
			list = append(list, p)
		}
	}
	_ = rows.Close()
	for _, p := range list {
		var dr domain.Downloader
		if err := database.Get(ctx, m.Store.Read(), "downloaders", p.downloader, &dr); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				m.removalDone(ctx, p.id)
			}
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			continue
		}
		deleteFiles, kept := p.pending != 0, ""
		if p.pending == removalCheckLater {
			var ok bool
			if kept, ok = m.laterKeepWhy(ctx, e, dr, p.external); !ok {
				continue
			}
			deleteFiles = kept == ""
		}
		if err := e.Remove(ctx, p.external, deleteFiles); err == nil || errors.Is(err, ErrNotFound) {
			m.removalDone(ctx, p.id)
			var d domain.Download
			if database.Get(ctx, m.Store.Read(), "downloads", p.id, &d) == nil {
				said, extra := "Removed from "+dr.Name+" on a later try", map[string]any{"delete_files": deleteFiles, "retried": true}
				if kept != "" {
					said += "; its files were kept: " + kept
					extra["kept"] = kept
				}
				m.eventWith(ctx, &d, "download_removed", said, extra)
			}
		}
	}
}
