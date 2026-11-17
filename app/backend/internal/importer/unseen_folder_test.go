package importer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// usenetNoList makes the download a Usenet job with no file list, as one whose folder HolyArrs could not read has.
func usenetNoList(t *testing.T, s *database.Store, dlID int64, savePath string) {
	t.Helper()
	if err := execSQL(s, "UPDATE downloaders SET protocol = 'usenet', adapter = 'sabnzbd'"); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(s, "UPDATE downloads SET files_seen = '[]', save_path = ? WHERE id = ?", savePath, dlID); err != nil {
		t.Fatal(err)
	}
}

// A show's Usenet job whose folder HolyArrs cannot see names the path pair and the mount, not a failed match.
func TestShowImportSaysAUsenetJobFolderIsNotThere(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	usenetNoList(t, w.s, w.dlID, filepath.Join(w.dl, "not-mounted", "job"))
	if _, err := (&Importer{Store: w.s, SavePath: w.dl}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != notThereWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// An album's Usenet job whose folder HolyArrs cannot see names the path pair and the mount, not a failed match.
func TestAlbumImportSaysAUsenetJobFolderIsNotThere(t *testing.T) {
	w := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	usenetNoList(t, w.s, w.dlID, filepath.Join(w.dl, "not-mounted", "job"))
	if _, err := (&Importer{Store: w.s, SavePath: w.dl}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), w.s.Read(), "slots", w.slots[0], &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != notThereWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// A movie whose download folder is not there says so even when the file's name matches nothing: matching by hand has no folder to open.
func TestMovieImportMissingFolderWinsOverNoMatch(t *testing.T) {
	w := setup(t)
	setFilesSeen(t, w, "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/group-sn.mkv")
	unseenFolder(t, w)
	wantNotThere(t, w, &Importer{Store: w.s})
}

// A show whose download folder is not there says so even when no file name matches an episode.
func TestShowImportMissingFolderWinsOverNoMatch(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	if err := execSQL(w.s, `UPDATE downloads SET files_seen = '[{"name":"abc/xyz.mkv","size":1000}]', save_path = ? WHERE id = ?`, filepath.Join(w.dl, "not-mounted"), w.dlID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != notThereWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// A job folder path that is not a full path here (a Windows path on Linux with no path pair) is never taken for files another app removed.
func TestSourcesGoneNeedsAFullPath(t *testing.T) {
	t.Chdir(t.TempDir())
	if sourcesGone("job", nil, true) || sourcesGone(filepath.Join("jobs", "job"), nil, true) {
		t.Fatal("a folder that is not a full path was taken as gone")
	}
}

// An episode file that goes away while it is placed, its folder staying, says the file is gone in words the app can show.
func TestShowImportFileGoneWhilePlacing(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	src := filepath.Join(w.dl, packName, ep(1))
	im := &Importer{Store: w.s, Ops: Ops{
		Link: func(_, _ string) error { _ = os.Remove(src); return &os.LinkError{Op: "link", Err: fs.ErrNotExist} },
		Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrNotExist} },
	}}
	if _, err := im.Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != fileTakenWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// A file whose name matches nothing in a download folder that is there keeps the no-match line.
func TestMovieImportNoMatchInAFolderThatIsThere(t *testing.T) {
	w := setup(t)
	setFilesSeen(t, w, "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/group-sn.mkv")
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if r := ptrText(slot308(t, w).LastReason); r != "no file in the download matched this movie; match the files by hand" {
		t.Fatalf("reason %q", r)
	}
}

// A show's Usenet job that was not seen is imported once its folder is there: the file list is stored and the waiting episode is woken, as Import again does.
func TestShowImportAfterAnUnseenJobIsThere(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	ctx := context.Background()
	usenetNoList(t, w.s, w.dlID, filepath.Join(w.dl, "not-mounted", "job"))
	im := &Importer{Store: w.s, SavePath: w.dl}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || !CanImportAgain(ptrText(sl.LastReason)) {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
	if err := execSQL(w.s, "UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', 2048)), save_path = ? WHERE id = ?", packName+"/"+ep(1), w.dl, w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing' WHERE state = 'needs_a_decision'"); err != nil {
		t.Fatal(err)
	}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.FileID == nil || sl.State == mediafiles.SlotNeedsADecision {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// An album's Usenet job that was not seen is imported once its folder is there.
func TestAlbumImportAfterAnUnseenJobIsThere(t *testing.T) {
	names := []string{"01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac"}
	w := albumSetup(t, 1, names...)
	ctx := context.Background()
	usenetNoList(t, w.s, w.dlID, filepath.Join(w.dl, "not-mounted", "job"))
	im := &Importer{Store: w.s, SavePath: w.dl}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if w.state(t, 0) != mediafiles.SlotNeedsADecision {
		t.Fatalf("slot %s", w.state(t, 0))
	}
	if err := execSQL(w.s, `UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', 2048), json_object('name', ?, 'size', 2048), json_object('name', ?, 'size', 2048)), save_path = ? WHERE id = ?`,
		albumRel+"/"+names[0], albumRel+"/"+names[1], albumRel+"/"+names[2], w.dl, w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing' WHERE state = 'needs_a_decision'"); err != nil {
		t.Fatal(err)
	}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	for i := range names {
		if w.state(t, i) != mediafiles.SlotInLibrary {
			t.Fatalf("track %d: %s", i+1, w.state(t, i))
		}
	}
}

// Import this file on a download whose video is gone meanwhile leaves that on the movie, not the reason of an earlier try.
func TestReviewImportWithNoFileLeftSaysSoOnTheMovie(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	setFilesSeen(t, w, "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP/group-sn.mkv")
	im := &Importer{Store: w.s}
	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State == "done" {
		t.Fatalf("auto import should not place it: %+v %v", imp, err)
	}
	if err := os.RemoveAll(filepath.Join(w.dl, "Some.Other.Name.2016.German.DL.1080p.BluRay.x264-GROUP")); err != nil {
		t.Fatal(err)
	}
	r, err := im.ImportReviewFile(ctx, w.slotID, w.dlID, false, domain.System)
	if err != nil || r.Outcome != "refused" || r.Error != ErrNoReviewFile.Error() {
		t.Fatalf("review import %+v %v", r, err)
	}
	if got := ptrText(slot308(t, w).LastReason); got != "the file could not be placed: "+ErrNoReviewFile.Error() {
		t.Fatalf("reason %q", got)
	}
}
