package downloads

import (
	"context"
	"errors"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// MoveAfterImport moves a fully imported torrent to its Downloader's category
// after import, once. Nothing moves on a read-only, paused or removed
// Downloader, nor while a season pack may still fetch more files. A torrent
// the client no longer holds is marked done without a History line; any
// other failure is written once and tried again on the next import run.
func (m *Manager) MoveAfterImport(ctx context.Context, downloadID int64) {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil || d.ReleaseID == nil || d.ExternalID == "" {
		return
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return
	}
	cat := dr.CategoryAfterImport
	if cat == "" || dr.Protocol != "torrent" || dr.Removed || dr.State == "disabled" || readOnlyFor(dr) || packStillOpen(d) {
		return
	}
	if moved, err := m.categoryMoved(ctx, d.ID); err != nil || moved {
		return
	}
	err := m.ControlEngine(ctx, dr, func(e Engine) error {
		mv, ok := e.(Mover)
		if !ok {
			return ErrNotSupported
		}
		// A torrent the client held in another category before it was picked keeps
		// the category its owner gave it.
		if j, err := e.Get(ctx, d.ExternalID); err == nil && !ownCategory(dr.Categories(), j.Category) {
			return outsideKept
		}
		return mv.SetCategory(ctx, d.ExternalID, cat)
	})
	switch {
	case errors.Is(err, outsideKept):
		_ = m.markCategoryMoved(ctx, d.ID)
	case err == nil:
		if m.markCategoryMoved(ctx, d.ID) == nil {
			m.event(ctx, &d, "download_changed", "Moved to "+cat+" after import")
		}
	case errors.Is(err, ErrNotFound):
		_ = m.markCategoryMoved(ctx, d.ID)
	default:
		m.event(ctx, &d, "download_not_moved", fmt.Sprintf("Could not move the imported download to %s in %s: %v; HolyArrs tries again on the next import", cat, dr.Name, firstLine(err.Error())))
	}
}
