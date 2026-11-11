package importer

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func usenetWorld(t *testing.T) world {
	t.Helper()
	w := setup(t)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET adapter = 'sabnzbd', protocol = 'usenet'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

func mayMove(domain.Downloader) bool { return true }

// mediaTemp is a download or Library folder for an import test. With
// HOLYARRS_TEST_MEDIA_DIR set it sits there instead, so the same tests run on a
// share that cannot hardlink (a Docker Desktop mount of a Windows folder) while
// the database stays on a local disk.
func mediaTemp(t *testing.T) string {
	t.Helper()
	base := os.Getenv("HOLYARRS_TEST_MEDIA_DIR")
	if base == "" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp(base, "media-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func placedFiles(t *testing.T, w world) []mediafiles.File {
	t.Helper()
	fs, err := database.Select[mediafiles.File](context.Background(), w.s.Read(), "files", "")
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestUsenetImportMoves(t *testing.T) {
	w := usenetWorld(t)
	src := filepath.Join(w.dl, relName+".mkv")
	imp, err := (&Importer{Store: w.s, MayMove: mayMove}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" || imp.Result[0].PlacedBy != "move" || imp.Result[0].FellBack {
		t.Fatalf("import %+v", imp)
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the download still holds the file")
	}
	files := placedFiles(t, w)
	if len(files) != 1 || files[0].PlacedBy != "move" {
		t.Fatalf("files %+v", files)
	}
	if fi, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(files[0].RelativePath))); err != nil || fi.Size() != 2048 {
		t.Fatalf("library file: %v", err)
	}
}

func TestTorrentAndReadOnlyImportsKeepTheDownload(t *testing.T) {
	for _, c := range []struct {
		name  string
		world func(*testing.T) world
		may   func(domain.Downloader) bool
	}{
		{"torrent", setup, mayMove},
		{"read-only Usenet", usenetWorld, func(domain.Downloader) bool { return false }},
		{"no rule", usenetWorld, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := c.world(t)
			imp, err := (&Importer{Store: w.s, MayMove: c.may}).Import(context.Background(), w.dlID, false, domain.System)
			if err != nil || imp.State != "done" || imp.Result[0].PlacedBy == "move" || imp.Plan[0].Move {
				t.Fatalf("import %+v %v", imp, err)
			}
			if _, err := os.Stat(filepath.Join(w.dl, relName+".mkv")); err != nil {
				t.Errorf("the download lost its file: %v", err)
			}
		})
	}
}

// A rename to another drive fails; the file is linked or copied and the download keeps it.
func TestMoveFallsBackToCopy(t *testing.T) {
	w := usenetWorld(t)
	src := filepath.Join(w.dl, relName+".mkv")
	im := &Importer{Store: w.s, MayMove: mayMove, Ops: Ops{
		Move: func(string, string) error { return &os.LinkError{Op: "rename", Err: syscall.EXDEV} },
		Link: func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} },
	}}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" || imp.Result[0].PlacedBy != "copy" || !imp.Result[0].FellBack || !strings.Contains(imp.Result[0].Error, "move not possible") {
		t.Fatalf("import %+v %v", imp, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("a copy took the file from the download: %v", err)
	}
}

// An attempt that moved the file but stopped before recording it is retried
// from the download folder: the moved file goes back, then moves again.
func TestMoveRetryAfterStop(t *testing.T) {
	w := usenetWorld(t)
	ctx := context.Background()
	src := filepath.Join(w.dl, relName+".mkv")
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TRIGGER no_lib BEFORE UPDATE OF state ON slots WHEN NEW.state = 'in_library' BEGIN SELECT RAISE(ABORT, 'boom'); END")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s, MayMove: mayMove}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); err == nil {
		t.Fatal("the first attempt should stop")
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("the first attempt did not move the file")
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DROP TRIGGER no_lib"); return err }); err != nil {
		t.Fatal(err)
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" || imp.Result[0].PlacedBy != "move" {
		t.Fatalf("retry %+v %v", imp, err)
	}
	if files := placedFiles(t, w); len(files) != 1 {
		t.Fatalf("files %+v", files)
	}
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	if sl.State != mediafiles.SlotInLibrary {
		t.Errorf("slot %s", sl.State)
	}
}

// A file at the target that does not match the recorded size and time is
// someone else's and never moves into the download folder.
func TestRestoreLeavesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "dl", "a.mkv"), filepath.Join(dir, "lib", "a.mkv")
	write(t, dst, 10)
	fi, _ := os.Stat(dst)
	prior := &mediafiles.Import{
		Plan:   []mediafiles.PlanStep{{SourceFile: src, TargetPath: dst, Method: "hardlink", Move: true}},
		Result: []mediafiles.PlaceResult{{SourceFile: src, Outcome: "placing", MoveSize: 11, MoveMod: fi.ModTime().UnixNano()}},
	}
	restoreMoves(prior)
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("another file was taken from the Library")
	}
	prior.Result[0].MoveSize = 10
	restoreMoves(prior)
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("the moved file was not put back: %v", err)
	}
}

func TestNoMoveFromInsideALibraryFolder(t *testing.T) {
	w := usenetWorld(t)
	inside := filepath.Join(w.lib, "incoming", "a.mkv")
	outside := filepath.Join(w.dl, "b.mkv")
	plan := []mediafiles.PlanStep{{SourceFile: inside, Method: "hardlink"}, {SourceFile: outside, Method: "hardlink"}, {SourceFile: outside, Method: "skip"}}
	(&Importer{Store: w.s, MayMove: mayMove}).markMoves(context.Background(), w.s.Read(), plan, domain.Downloader{Protocol: "usenet"}, nil)
	if plan[0].Move || !plan[1].Move || plan[2].Move {
		t.Fatalf("plan %+v", plan)
	}
	torrent := []mediafiles.PlanStep{{SourceFile: outside, Method: "hardlink"}, {SourceFile: filepath.Join(w.dl, ".holyarrs-unpack-1", "c.mkv"), Method: "hardlink"}}
	(&Importer{Store: w.s, MayMove: mayMove}).markMoves(context.Background(), w.s.Read(), torrent, domain.Downloader{Protocol: "torrent"}, []Candidate{{Path: torrent[1].SourceFile}})
	if torrent[0].Move || !torrent[1].Move {
		t.Fatalf("torrent plan %+v", torrent)
	}
}

// A Usenet season pack stopped right after its files moved is finished by the
// next attempt, with every episode recorded once.
func TestShowUsenetMoveResumesAfterCrash(t *testing.T) {
	w := showSetup(t, []int{1, 2, 3}, ep(1), ep(2), ep(3))
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET adapter = 'sabnzbd', protocol = 'usenet'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	crashAt = func(stage string) error {
		if stage == "placed" {
			return errors.New("killed")
		}
		return nil
	}
	im := &Importer{Store: w.s, MayMove: mayMove}
	_, err := im.Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("resume %v %+v", err, imp)
	}
	for _, r := range imp.Result {
		if r.PlacedBy != "move" {
			t.Errorf("result %+v", r)
		}
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE placed_by = 'move'").Scan(&n)
	if n != 3 {
		t.Fatalf("files %d", n)
	}
	left, _ := os.ReadDir(w.dl)
	for _, e := range left {
		if strings.HasSuffix(e.Name(), ".mkv") {
			t.Errorf("left in the download: %s", e.Name())
		}
	}
}

// A Usenet upgrade under a new file name that stops after the late hold still
// has its move recorded, so the next attempt finishes it with one Library file.
func TestUsenetUpgradeSurvivesCrashAfterLateHold(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		"Dune.Part.Two.2024.720p.HDTV.x264-OTHER")
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET adapter = 'sabnzbd', protocol = 'usenet'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	crashAt = func(s string) error {
		if s == "held_late" {
			return os.ErrDeadlineExceeded
		}
		return nil
	}
	im := &Importer{Store: w.s, MayMove: mayMove}
	_, err := im.Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" || imp.Result[0].PlacedBy != "move" {
		t.Fatalf("restart %+v %v", imp, err)
	}
	var old mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", oldID, &old)
	var present int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'present'").Scan(&present)
	if old.State != "replaced" || present != 1 {
		t.Fatalf("old %s, present files %d", old.State, present)
	}
}
