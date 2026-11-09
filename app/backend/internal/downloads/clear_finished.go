package downloads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

var jobSuffix = regexp.MustCompile(`\.\d{1,3}$`)

// ClearFinished removes finished jobs from the Downloader that downloadID names.
func (m *Manager) ClearFinished(ctx context.Context, downloadID int64) {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil || d.ReleaseID == nil || d.ExternalID == "" {
		return
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return
	}
	if dr.Protocol != "usenet" || !dr.ClearFinished || readOnlyFor(dr) {
		return
	}
	e, err := m.engine(ctx, dr)
	var folder string
	if err == nil {
		if j, gerr := e.Get(ctx, d.ExternalID); gerr == nil {
			folder = j.ContentPath
		}
		err = e.Remove(ctx, d.ExternalID, true)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Could not clear the imported download from %s: %v", dr.Name, firstLine(err.Error())))
		return
	}
	var relName string
	var rel domain.Release
	if database.Get(ctx, m.Store.Read(), "releases", *d.ReleaseID, &rel) == nil {
		relName = rel.Name
	}
	said := "Cleared the imported download from " + dr.Name
	if root, rel := m.finishedFolder(folder, relName, dr); rel != "" {
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

func (m *Manager) finishedFolder(content, relName string, dr domain.Downloader) (root, rel string) {
	if content == "" || relName == "" {
		return "", ""
	}
	local := filepath.Clean(mapLocal(content, dr.PathMap))
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

func sameJobName(folder, relName string) bool {
	return strings.EqualFold(folder, relName) || strings.EqualFold(jobSuffix.ReplaceAllString(folder, ""), relName)
}
