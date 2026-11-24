package downloads

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Removing a finished download by hand: a torrent still seeding, or a job the
// Downloader still lists after the import. Only HolyArrs' copy in the
// Downloader goes; the Library copy is never touched.

var (
	// ErrImporting means HolyArrs is placing the download's files right now.
	ErrImporting = errors.New("HolyArrs is putting this download's files in the library; remove it once that is done")
	// ErrDownloaderReadOnly means the Downloader is set so HolyArrs only reads it.
	ErrDownloaderReadOnly = errors.New(`this Downloader is read-only: turn on "HolyArrs can change this Downloader" in its settings to remove it from here`)
)

func finishedState(state string) bool { return state == "finished" || state == "seeding" }

// doneRemovable checks a finished download may be removed by hand, and
// changes c so it never undoes the import: an imported download's Release is
// not turned down and nothing is searched again.
func (m *Manager) doneRemovable(ctx context.Context, dr domain.Downloader, d domain.Download, c *RemoveChoice) error {
	if readOnlyFor(dr) {
		return ErrDownloaderReadOnly
	}
	if running, err := mediafiles.ImportRunning(ctx, m.Store.Read(), d.ID); err != nil {
		return err
	} else if running {
		return ErrImporting
	}
	if settled, err := mediafiles.ImportSettled(ctx, m.Store.Read(), d.ID, d.SlotIDs); err != nil {
		return err
	} else if settled {
		c.TurnDown, c.SearchAgain = false, false
	}
	return nil
}

// doneKeepWhy says why a finished download's files must stay although the
// person asked to delete them, or "": they are in or around a Library folder,
// they are a whole save folder, or another torrent uses them. A Library copy
// that is a hard link of a download file is a second name for the same data,
// so deleting the download's name leaves it whole. later is true when the
// Downloader could not be asked; the check then runs again on the later try.
func (m *Manager) doneKeepWhy(ctx context.Context, dr domain.Downloader, d domain.Download) (why string, later bool) {
	e, err := m.engine(ctx, dr)
	if err != nil {
		return "", true
	}
	j, err := e.Get(ctx, d.ExternalID)
	if errors.Is(err, ErrNotFound) {
		return "", false
	}
	if err != nil {
		return "", true
	}
	return m.keepFilesWhy(ctx, e, dr, j), false
}

// removalCheckLater is the removal_pending value for files the person asked
// to delete that may only go once the Downloader says where they are.
const removalCheckLater = 2

// laterKeepWhy runs the keep check for a removal queued with
// removalCheckLater. ok is false while the Downloader still cannot say.
func (m *Manager) laterKeepWhy(ctx context.Context, e Engine, dr domain.Downloader, external string) (why string, ok bool) {
	j, err := e.Get(ctx, external)
	if errors.Is(err, ErrNotFound) {
		return "", true
	}
	if err != nil {
		return "", false
	}
	return m.keepFilesWhy(ctx, e, dr, j), true
}

// inLibrary says files at local are a Library folder or are in or around one.
func inLibrary(local string, folders []string) bool {
	for _, k := range folders {
		if k = strings.TrimSpace(k); k != "" && (underAny(local, []string{k}) || underAny(filepath.Clean(k), []string{local})) {
			return true
		}
	}
	return false
}

// markFilesStay marks HolyArrs' own finished rows whose files Remove keeps
// because they are in or around a Library folder, so the confirm says so.
func (a *Web) markFilesStay(ctx context.Context, rows []DownloadRow, names map[int64]domain.Downloader) {
	ask := func(r DownloadRow) bool { return !r.ClientItem && r.Path != "" && finishedState(r.State) }
	if !slices.ContainsFunc(rows, ask) {
		return // no read when no row needs it
	}
	fs, err := database.Select[mediafiles.LibraryFolder](ctx, a.read(), "library_folders", "stand_in = 0")
	if err != nil {
		return
	}
	folders := make([]string, 0, len(fs))
	for _, f := range fs {
		folders = append(folders, f.Path)
	}
	for i := range rows {
		r := &rows[i]
		if ask(*r) {
			r.FilesStay = inLibrary(filepath.Clean(mapLocal(r.Path, names[r.DownloaderID].PathMap)), folders)
		}
	}
}
