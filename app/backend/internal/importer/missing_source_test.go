package importer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// unseenFolder points the download at a save folder HolyArrs cannot see, as a wrong path pair or an empty mount does.
func unseenFolder(t *testing.T, w world) {
	t.Helper()
	if err := execSQL(w.s, "UPDATE downloads SET save_path = ? WHERE id = ?", filepath.Join(w.dl, "not-mounted"), w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
}

func wantNotThere(t *testing.T, w world, im *Importer) {
	t.Helper()
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State == "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != notThereWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), notThereWhy)
	}
}

// A movie whose downloaded file is not where the Downloader says says so, not that no file matched or that another app took it.
func TestMovieImportSaysTheFileIsNotWhereTheDownloaderSays(t *testing.T) {
	for _, tiny := range []string{"true", "false"} {
		t.Run("refuse tiny releases "+tiny, func(t *testing.T) {
			w := setup(t)
			setFake(t, w.s, "refuse_tiny_releases", tiny)
			unseenFolder(t, w)
			wantNotThere(t, w, &Importer{Store: w.s})
		})
	}
}

// A missing file on a Version that requires audio languages is not blamed on its audio.
func TestMovieImportDoesNotBlameAudioForAMissingFile(t *testing.T) {
	w := setup(t)
	setVersionAudio(t, w, "fr")
	unseenFolder(t, w)
	wantNotThere(t, w, &Importer{Store: w.s})
}

// A download folder that goes away while the file is placed says the file is not there, not the system's words.
func TestMovieImportSaysTheFileWentAwayWithItsFolder(t *testing.T) {
	w := setup(t)
	setFake(t, w.s, "refuse_tiny_releases", "false")
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	wantNotThere(t, w, &Importer{Store: w.s, Ops: Ops{
		Link: func(_, _ string) error {
			if err := os.RemoveAll(w.dl); err != nil {
				t.Error(err)
			}
			return &os.LinkError{Op: "link", Err: fs.ErrNotExist}
		},
		Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrNotExist} },
	}})
}

func wantTaken(t *testing.T, w world, im *Importer) {
	t.Helper()
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State == "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != fileTakenWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), fileTakenWhy)
	}
}

// A listed movie file that has not reached the local download folder yet gets
// a waiting message, while a file that vanishes during placement keeps the
// existing missing-file message below.
func TestMovieImportSaysFileIsNotPresentYet(t *testing.T) {
	w := setup(t)
	if err := os.Remove(filepath.Join(w.dl, relName+".mkv")); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != NotPresentYetWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), NotPresentYetWhy)
	}
}

// A synced file that exists but is shorter than the Downloader's file list is
// still arriving. Keep it pending instead of importing or rejecting it.
func TestMovieImportWaitsForAPartlyCopiedFile(t *testing.T) {
	w := setup(t)
	src := filepath.Join(w.dl, relName+".mkv")
	if err := os.Truncate(src, 512); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != CopyingWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), CopyingWhy)
	}
	if entries, err := os.ReadDir(w.lib); err != nil || len(entries) != 0 {
		t.Fatalf("Library changed: %v %v", entries, err)
	}
}

// A movie whose video is gone while its folder and the files beside it stay does not blame the path pair or the mount:
// only video or audio files count, so another app took the Usenet job's video.
func TestMovieImportVideoGoneWhileItsFolderStays(t *testing.T) {
	for _, tiny := range []string{"true", "false"} {
		t.Run("refuse tiny releases "+tiny, func(t *testing.T) {
			w := setup(t)
			// A Usenet job: another app moves a job's files out (a torrent waits instead).
			if err := execSQL(w.s, "UPDATE downloaders SET protocol = 'usenet', adapter = 'sabnzbd'"); err != nil {
				t.Fatal(err)
			}
			setFake(t, w.s, "refuse_tiny_releases", tiny)
			setFilesSeen(t, w, relName+".mkv", relName+".nfo")
			if err := os.Remove(filepath.Join(w.dl, relName+".mkv")); err != nil {
				t.Fatal(err)
			}

			if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
				t.Fatal(err)
			}
			if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
				t.Fatal(err)
			}
			if sl := slot308(t, w); ptrText(sl.LastReason) != FilesGoneReason {
				t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), FilesGoneReason)
			}
		})
	}
}

// A video that goes away while it is placed, its folder staying, does not blame the path pair or the mount.
func TestMovieImportVideoGoneWhilePlacing(t *testing.T) {
	w := setup(t)
	src := filepath.Join(w.dl, relName+".mkv")
	wantTaken(t, w, &Importer{Store: w.s, Ops: Ops{
		Link: func(_, _ string) error {
			if err := os.Remove(src); err != nil {
				t.Error(err)
			}
			return &os.LinkError{Op: "link", Err: fs.ErrNotExist}
		},
		Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrNotExist} },
	}})
}

// A Usenet job whose folder HolyArrs cannot see gives no files to match: the line names the path pair and the mount, not a failed match.
func TestMovieImportSaysAUsenetJobFolderIsNotThere(t *testing.T) {
	w := setup(t)
	if err := execSQL(w.s, "UPDATE downloaders SET protocol = 'usenet', adapter = 'sabnzbd'"); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE downloads SET files_seen = '[]', save_path = ? WHERE id = ?", filepath.Join(w.dl, "not-mounted", "job"), w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	wantNotThere(t, w, &Importer{Store: w.s, SavePath: w.dl})
}

// A missing episode file on a Version that requires audio languages is not blamed on its audio.
func TestShowImportDoesNotBlameAudioForAMissingFile(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	setShowVersionAudio(t, w, "fr")
	if err := os.Remove(filepath.Join(w.dl, packName, ep(1))); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	sl := w.slot(t, 1)
	if r := ptrText(sl.LastReason); sl.State != mediafiles.SlotNeedsADecision || r != NotPresentYetWhy {
		t.Fatalf("slot %s reason %q", sl.State, r)
	}
}

// An episode's refusal for a file that is gone or cannot be read is the lead and that one reason, so it reads in the picked language; other refusals keep every part.
func TestEpisodeRefusedWhyKeepsOnlyTheReadingReason(t *testing.T) {
	root := t.TempDir()
	match := []string{"Copper Valley by clean name", "holds S01E01"}
	for why, want := range map[string]string{
		NotPresentYetWhy: NotPresentYetWhy,
		CopyingWhy:       CopyingWhy,
		notInFolderWhy:   "the file for S01E01 could not be placed: " + notInFolderWhy,
		unreadCheckWhy:   "the file for S01E01 could not be placed: " + unreadCheckWhy,
		noReadCheckWhy:   noReadWhy,
		"a sample, not the full video (it plays 1 minute)": "the file for S01E01 could not be placed: Copper Valley by clean name; holds S01E01; a sample, not the full video (it plays 1 minute)",
	} {
		if got := episodeRefusedWhy(root, "S01E01", append(slices.Clone(match), why)); got != want {
			t.Errorf("%q: %q, want %q", why, got, want)
		}
	}
}

// An episode whose download folder HolyArrs cannot see names the path pair and the mount, as a movie does.
func TestShowImportSaysTheFolderIsNotThere(t *testing.T) {
	for _, audio := range []bool{true, false} {
		w := showSetup(t, []int{1}, ep(1))
		if audio {
			setShowVersionAudio(t, w, "fr")
		}
		if err := execSQL(w.s, "UPDATE downloads SET save_path = ? WHERE id = ?", filepath.Join(w.dl, "not-mounted"), w.dlID); err != nil {
			t.Fatal(err)
		}
		if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
			t.Fatal(err)
		}
		if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != notThereWhy {
			t.Fatalf("audio %v: slot %s reason %q", audio, sl.State, ptrText(sl.LastReason))
		}
	}
}

// An episode whose download folder goes away while its file is placed names the path pair and the mount.
func TestShowImportSaysTheFolderWentAwayWhilePlacing(t *testing.T) {
	w := showSetup(t, []int{1}, ep(1))
	im := &Importer{Store: w.s, Ops: Ops{
		Link: func(_, _ string) error {
			if err := os.RemoveAll(w.dl); err != nil {
				t.Error(err)
			}
			return &os.LinkError{Op: "link", Err: fs.ErrNotExist}
		},
		Copy: func(src, _ string) error { return &fs.PathError{Op: "open", Path: src, Err: fs.ErrNotExist} },
	}}
	if _, err := im.Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != notThereWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// A download with no save folder and no file list keeps the no-match line.
func TestMovieImportWithNoSaveFolderKeepsTheNoMatchLine(t *testing.T) {
	w := setup(t)
	if err := execSQL(w.s, "UPDATE downloads SET files_seen = '[]', save_path = '' WHERE id = ?", w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s, SavePath: w.dl}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := slot308(t, w); ptrText(sl.LastReason) != "no file in the download matched this movie; match the files by hand" {
		t.Fatalf("reason %q", ptrText(sl.LastReason))
	}
}

// The line names where to look and keeps the word that sorts it as a stuck import.
func TestNotThereLineSaysWhereToLook(t *testing.T) {
	for _, part := range []string{"path pair", "Settings > Downloaders", "mounted", "import again"} {
		if !strings.Contains(notThereWhy, part) {
			t.Fatalf("%q lacks %q", notThereWhy, part)
		}
	}
}
