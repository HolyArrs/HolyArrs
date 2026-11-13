package downloads

import (
	"context"
	"errors"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrOutsideTorrent means the chosen Release's torrent is already in the
// Downloader in a category that is not HolyArrs's, and the Downloader is set to
// show and change only HolyArrs's own items, so HolyArrs does not take it.
var ErrOutsideTorrent = errors.New("the Downloader already has this torrent outside HolyArrs, and Show only HolyArrs items is on: move it to HolyArrs's category there, or turn that setting off")

// outsideKept stops the move after import of a torrent outside HolyArrs's
// categories.
var outsideKept = errors.New("the torrent is outside HolyArrs's categories")

// outsideJob finds a followed torrent that the read of HolyArrs's categories
// left out: one qBittorrent already held in another category (or none) when its
// Release was chosen, which the Download follows by info hash. The background
// reader's full snapshot holds it; a direct read asks for it by hash.
func outsideJob(ctx context.Context, e Engine, dr domain.Downloader, v SyncView, fromStore bool, id string) (Job, bool) {
	if dr.Adapter != "qbittorrent" || id == "" {
		return Job{}, false
	}
	if fromStore {
		j, ok := v.Items[id]
		return j, ok && v.Scope == ""
	}
	j, err := e.Get(ctx, id)
	return j, err == nil
}
