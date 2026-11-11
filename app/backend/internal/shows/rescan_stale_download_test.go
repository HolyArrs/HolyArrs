package shows

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// stuckOnDownload ties S02E03 of an adopted show to a download in the given
// state and parks the episode in needs_a_decision, as an import whose files
// another app took did before.
func stuckOnDownload(t *testing.T, state string) (*Service, *database.Store, int64, int64, string) {
	t.Helper()
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	slotID := episodes(t, s, res[0].TitleID)["S02E03"].SlotID
	var dlID int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "sab", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://sab", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, SlotIDs: []int64{slotID}, ExternalID: "job", Progress: 1, State: state, FilesSeen: []domain.SeenFile{},
			SavePath: filepath.Join(lf.Path, "gone"), StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		dlID = dl.ID
		_, err := tx.Exec("UPDATE slots SET state = 'needs_a_decision', active_download_id = ?, last_reason = 'the file could not be placed: lstat /mnt/x.mkv: no such file or directory' WHERE id = ?", dl.ID, slotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab Show - S02E03 - Third.mkv"))
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "placed by another app")
	return svc, s, res[0].TitleID, dlID, rel
}

// The other app's file links on a rescan once the episode's download is no
// longer running, and the stale download is let go.
func TestRescanLinksEpisodeOfFinishedDownload(t *testing.T) {
	svc, s, titleID, _, rel := stuckOnDownload(t, "finished")
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	ep := episodes(t, s, titleID)["S02E03"]
	var origin string
	_ = s.Read().QueryRow("SELECT origin FROM files WHERE relative_path = ?", rel).Scan(&origin)
	var active sql.NullInt64
	_ = s.Read().QueryRow("SELECT active_download_id FROM slots WHERE id = ?", ep.SlotID).Scan(&active)
	if origin != "adopted" || active.Valid || (ep.State != mediafiles.SlotInLibrary && ep.State != mediafiles.SlotBetterWanted) {
		t.Fatalf("origin %q, active download %v, episode %s", origin, active, ep.State)
	}
}

// A download still running keeps its episode: the found file stays a note.
func TestRescanLeavesEpisodeOfRunningDownload(t *testing.T) {
	svc, s, titleID, dlID, rel := stuckOnDownload(t, "downloading")
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	ep := episodes(t, s, titleID)["S02E03"]
	var origin string
	_ = s.Read().QueryRow("SELECT origin FROM files WHERE relative_path = ?", rel).Scan(&origin)
	var active sql.NullInt64
	_ = s.Read().QueryRow("SELECT active_download_id FROM slots WHERE id = ?", ep.SlotID).Scan(&active)
	if origin != "found" || !active.Valid || active.Int64 != dlID || ep.State != mediafiles.SlotNeedsADecision {
		t.Fatalf("origin %q, active download %v, episode %s", origin, active, ep.State)
	}
}
