package downloader

import (
	"context"
	"errors"
	"fmt"

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
	if err == nil {
		err = e.Remove(ctx, d.ExternalID, true)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Could not clear the imported download from %s: %v", dr.Name, firstLine(err.Error())))
		return
	}
	m.event(ctx, &d, "download_removed", "Cleared the imported download and its files from "+dr.Name)
}
