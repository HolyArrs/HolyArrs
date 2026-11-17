//go:build unix

package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A download folder HolyArrs may read but not search says the file may not be read, not the system's words.
func TestPlaceSaysUnsearchableSourceMayNotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	dir := filepath.Join(t.TempDir(), "dl")
	src := filepath.Join(dir, "a.mkv")
	write(t, src, 64)
	if err := os.Chmod(dir, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	r := place(mediafiles.PlanStep{Method: "hardlink", SourceFile: src, TargetPath: filepath.Join(t.TempDir(), "m", "a.mkv")}, Ops{}, time.Time{})
	if r.Outcome != "failed" || r.Error != noReadWhy || strings.Contains(r.Error, "lstat") {
		t.Fatalf("%s %q", r.Outcome, r.Error)
	}
}

// A movie whose download folder HolyArrs may not search says the file may not be read, not that no file matched.
func TestMovieImportSaysUnsearchableFolderMayNotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	w := setup(t)
	ctx := context.Background()
	setFilesSeen(t, w, relName+"/"+relName+".mkv")
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(w.dl, relName)
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State == "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != noReadWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), noReadWhy)
	}
}

// A file HolyArrs may not open on a Version that requires audio languages says the file may not be read, not that its audio falls short.
func TestMovieImportDoesNotBlameAudioForAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	w := setup(t)
	ctx := context.Background()
	setVersionAudio(t, w, "fr")
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(w.dl, relName+".mkv"), 0); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State == "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != noReadWhy {
		t.Fatalf("slot %s reason %q, want %q", sl.State, ptrText(sl.LastReason), noReadWhy)
	}
}

// A file HolyArrs may not read but can still place is imported, not left waiting.
func TestMovieImportPlacesAFileItMayNotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	w := setup(t)
	setFake(t, w.s, "refuse_tiny_releases", "false")
	if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(w.dl, relName+".mkv"), 0); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %+v %v", imp, err)
	}
}

// An episode file HolyArrs may not open on a Version that requires audio languages says the file may not be read.
func TestShowImportDoesNotBlameAudioForAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	w := showSetup(t, []int{1}, ep(1))
	setShowVersionAudio(t, w, "fr")
	if err := os.Chmod(filepath.Join(w.dl, packName, ep(1)), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	sl := w.slot(t, 1)
	if r := ptrText(sl.LastReason); sl.State != mediafiles.SlotNeedsADecision || r != noReadWhy {
		t.Fatalf("slot %s reason %q", sl.State, r)
	}
}

// A Usenet job folder HolyArrs may not list gives no files to match: the line names the read access, not a failed match.
func TestImportSaysAJobFolderItMayNotList(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	t.Run("movie", func(t *testing.T) {
		w := setup(t)
		job := filepath.Join(w.dl, "job")
		if err := os.Mkdir(job, 0); err != nil {
			t.Fatal(err)
		}
		usenetNoList(t, w.s, w.dlID, job)
		if err := execSQL(w.s, "UPDATE slots SET active_download_id = ? WHERE id = ?", w.dlID, w.slotID); err != nil {
			t.Fatal(err)
		}
		if _, err := (&Importer{Store: w.s, SavePath: w.dl}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
			t.Fatal(err)
		}
		if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != noReadWhy {
			t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
		}
	})
	t.Run("show", func(t *testing.T) {
		w := showSetup(t, []int{1}, ep(1))
		job := filepath.Join(w.dl, "job")
		if err := os.Mkdir(job, 0); err != nil {
			t.Fatal(err)
		}
		usenetNoList(t, w.s, w.dlID, job)
		if _, err := (&Importer{Store: w.s, SavePath: w.dl}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
			t.Fatal(err)
		}
		if sl := w.slot(t, 1); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != noReadWhy {
			t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
		}
	})
}
