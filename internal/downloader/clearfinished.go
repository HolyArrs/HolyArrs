package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// ClearFinished takes a Usenet download out of SABnzbd or NZBGet, finished
// job and files, once its Import fully succeeded (the caller says so). It
// leaves alone torrents, downloads HolyArrs did not send, Downloaders that
// are read only here, and Downloaders with "clear_finished" off. A client
// that refuses is logged in History and the Import stays done.
func (m *Manager) ClearFinished(ctx context.Context, downloadID int64) {
	var d domain.Download
	if err := store.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil || d.ReleaseID == nil || d.ExternalID == "" {
		return
	}
	var dr domain.Downloader
	if err := store.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return
	}
	if dr.Protocol != "usenet" || !dr.ClearFinished || readOnlyFor(dr) {
		return
	}
	e, err := m.engine(ctx, dr)
	var folder string
	if err == nil {
		// Where the client put the finished job, read before it forgets it.
		if j, gerr := e.Get(ctx, d.ExternalID); gerr == nil {
			folder = j.ContentPath
		}
		err = e.Remove(ctx, d.ExternalID, true)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Could not clear the imported download from %s: %v", dr.Name, firstLine(err.Error())))
		return
	}
	// SABnzbd and NZBGet drop a finished job from their list but may leave
	// its output folder: HolyArrs removes it itself, only inside a folder
	// the Downloader shares with it (its path map, or the save root).
	if left := m.finishedFolder(folder, dr); left != "" {
		if rerr := os.RemoveAll(left); rerr != nil {
			m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Cleared the imported download from %s, but its files could not be deleted: %v", dr.Name, firstLine(rerr.Error())))
			return
		}
	}
	m.event(ctx, &d, "download_removed", "Cleared the imported download and its files from "+dr.Name)
}

// finishedFolder is the job's folder as HolyArrs sees it, when it may be
// deleted: below (never equal to) one of the Downloader's shared folders,
// still there, and not a link. "" means leave the disk alone.
func (m *Manager) finishedFolder(content string, dr domain.Downloader) string {
	if content == "" {
		return ""
	}
	local := filepath.Clean(mapLocal(content, dr.PathMap))
	var roots []string
	if m.SavePath != "" {
		roots = append(roots, m.SavePath)
	}
	for _, pm := range dr.PathMap {
		roots = append(roots, pm.Ours)
	}
	inside := false
	for _, r := range roots {
		if r = filepath.Clean(r); r != "" && r != "." && local != r && underAny(local, []string{r}) {
			inside = true
		}
	}
	if !inside {
		return ""
	}
	fi, err := os.Lstat(local)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	return local
}
