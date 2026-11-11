package library

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// movieWithFoundCopy adds a movie whose slot has no file, ties it to a
// download in the given state (none when state is empty) and puts a video
// in its folder, as another app that imported the download does.
func movieWithFoundCopy(t *testing.T, state string) (*Service, *database.Store, int64, string) {
	t.Helper()
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	v := versionsOf(t, s, id)[0]
	slotID := slotsOf(t, s, id)[0].ID
	if state != "" {
		err = s.Write(ctx, func(tx *sql.Tx) error {
			d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
			if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
				return err
			}
			dl := &domain.Download{DownloaderID: d.ID, SlotIDs: []int64{slotID}, ExternalID: "abc", Progress: 1, State: state, FilesSeen: []domain.SeenFile{},
				SavePath: filepath.Join(lf.Path, "gone"), StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE slots SET state = 'needs_a_decision', active_download_id = ? WHERE id = ?", dl.ID, slotID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rel := v.FolderName + "/Dune.Part.Two.2024.1080p.WEB-DL.H264-GRP.mkv"
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "placed by another app")
	running := func(tx *sql.Tx, dlID int64) (bool, error) {
		var st string
		if err := tx.QueryRow("SELECT state FROM downloads WHERE id = ?", dlID).Scan(&st); err != nil {
			return false, err
		}
		return st == "downloading", nil
	}
	return &Service{Store: s, Now: func() time.Time { return now }, DownloadRunning: running}, s, id, rel
}

func linkedCopy(t *testing.T, s *database.Store, titleID int64, rel string) (mediafiles.Slot, string) {
	t.Helper()
	sl := slotsOf(t, s, titleID)[0]
	var origin string
	_ = s.Read().QueryRow("SELECT origin FROM files WHERE relative_path = ?", rel).Scan(&origin)
	return sl, origin
}

// After an import let its download go because another app took the files,
// the movie is wanted with no download: the rescan links the copy that app
// placed, so the movie is not searched again.
func TestRescanLinksMovieCopyOfWantedMovie(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "")
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	sl, origin := linkedCopy(t, s, id, rel)
	if sl.FileID == nil || sl.State != mediafiles.SlotInLibrary || origin != "adopted" {
		t.Fatalf("slot %+v, origin %q", sl, origin)
	}
}

// A movie stuck from an older version on a download that is no longer
// running is linked too, and the stale download is let go.
func TestRescanLinksMovieOfFinishedDownload(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "finished")
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	sl, origin := linkedCopy(t, s, id, rel)
	if sl.FileID == nil || sl.ActiveDownloadID != nil || sl.State != mediafiles.SlotInLibrary || origin != "adopted" {
		t.Fatalf("slot %+v, origin %q", sl, origin)
	}
}

// A download still running keeps its movie: the found file stays a note.
func TestRescanLeavesMovieOfRunningDownload(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "downloading")
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	sl, origin := linkedCopy(t, s, id, rel)
	if sl.FileID != nil || sl.ActiveDownloadID == nil || sl.State != mediafiles.SlotNeedsADecision || origin != "found" {
		t.Fatalf("slot %+v, origin %q", sl, origin)
	}
}
