package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	var relName string
	var rel domain.Release
	if store.Get(ctx, m.Store.Read(), "releases", *d.ReleaseID, &rel) == nil {
		relName = rel.Name
	}
	said := "Cleared the imported download from " + dr.Name
	if root, rel := m.finishedFolder(folder, relName, dr); rel != "" {
		// Deleted through a handle on the shared folder: os.Root never follows a
		// link or junction out of it, whatever the path's parts point at.
		rerr := func() error {
			rt, err := os.OpenRoot(root)
			if err != nil {
				return err
			}
			defer rt.Close()
			return rt.RemoveAll(rel)
		}()
		if rerr != nil {
			m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Cleared the imported download from %s, but its files could not be deleted: %v", dr.Name, firstLine(rerr.Error())))
			return
		}
		said = "Cleared the imported download and its files from " + dr.Name
	}
	m.event(ctx, &d, "download_removed", said)
}

// finishedFolder says which shared folder holds the job's folder and the
// folder's path inside it, when it may be deleted: named after this
// download's Release (a client's ".1" suffix allowed), below (never equal
// to) one of the Downloader's shared folders (a file's own folder when the
// client names the job by its file) (path map or save root), there,
// and a plain folder (not a link, junction or other special file). Anything
// else, such as a category folder the client named instead, is left alone:
// rel "" means leave the disk alone.
func (m *Manager) finishedFolder(content, relName string, dr domain.Downloader) (root, rel string) {
	if content == "" || relName == "" {
		return "", ""
	}
	local := filepath.Clean(mapLocal(content, dr.PathMap))
	// SABnzbd names a one-file job by its file: the job folder is its parent.
	if fi, err := os.Lstat(local); err == nil && fi.Mode().IsRegular() {
		local = filepath.Dir(local)
	}
	if !sameJobName(filepath.Base(local), relName) {
		return "", ""
	}
	fi, err := os.Lstat(local)
	if err != nil || !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return "", ""
	}
	var roots []string
	if m.SavePath != "" {
		roots = append(roots, m.SavePath)
	}
	for _, pm := range dr.PathMap {
		roots = append(roots, pm.Ours)
	}
	for _, r := range roots {
		if r = filepath.Clean(r); r == "" || r == "." || r == local || !underAny(local, []string{r}) {
			continue
		}
		if p, err := filepath.Rel(r, local); err == nil && p != "." && !filepath.IsAbs(p) {
			return r, p
		}
	}
	return "", ""
}

var jobSuffix = regexp.MustCompile(`\.\d{1,3}$`)

// sameJobName says whether a folder is named after the Release: equal
// ignoring case, or with the ".1"-style suffix a client adds on a clash.
func sameJobName(folder, relName string) bool {
	return strings.EqualFold(folder, relName) || strings.EqualFold(jobSuffix.ReplaceAllString(folder, ""), relName)
}
