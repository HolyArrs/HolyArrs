package importer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestSafeNameSuffixesWindowsDeviceNames(t *testing.T) {
	for in, want := range map[string]string{
		"CON": "CON_", "con": "con_", "Nul.txt": "Nul_.txt", "COM1": "COM1_", "lpt9.mkv": "lpt9_.mkv", "aux ": "aux_",
		"COMPUTER": "COMPUTER", "Con Air (1997)": "Con Air (1997)", "COM10": "COM10", "LPT0": "LPT0",
	} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := FolderName("Con", 0); got != "Con_" {
		t.Fatalf("folder %q", got)
	}
}

func TestImportRefusesCopyWithoutFreeSpace(t *testing.T) {
	w := setup(t)
	freeSpaceOf = func(string) int64 { return 10 }
	t.Cleanup(func() { freeSpaceOf = FreeSpace })
	im := &Importer{Store: w.s, Ops: Ops{Link: func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }}}
	imp, err := im.Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "failed" || imp.Result[0].Outcome != "refused" || !strings.Contains(imp.Result[0].Error, "not enough free space") {
		t.Fatalf("import %s %+v", imp.State, imp.Result)
	}
	if ents, _ := os.ReadDir(filepath.Join(w.lib, "Dune - Part Two (2024)")); len(ents) != 0 {
		t.Fatalf("something was written: %v", ents)
	}
	sl := slotOf(t, w)
	var reason string
	_ = w.s.Read().QueryRow("SELECT COALESCE(last_reason, '') FROM slots WHERE id = ?", w.slotID).Scan(&reason)
	if sl.State != mediafiles.SlotNeedsADecision || !strings.Contains(reason, "not enough free space") {
		t.Fatalf("slot %s %q", sl.State, reason)
	}
}

func TestCheckSpace(t *testing.T) {
	freeSpaceOf = func(string) int64 { return MinFreeSpace + 100 }
	t.Cleanup(func() { freeSpaceOf = FreeSpace })
	if err := CheckSpace(t.TempDir(), 100); err != nil {
		t.Fatal(err)
	}
	if err := CheckSpace(t.TempDir(), 101); err == nil || !strings.Contains(err.Error(), "not enough free space") {
		t.Fatalf("err %v", err)
	}
	freeSpaceOf = func(string) int64 { return -1 }
	if err := CheckSpace(t.TempDir(), 1<<40); err != nil {
		t.Fatalf("unknown free space must not block: %v", err)
	}
}

func TestFreeSpaceIsKnownHere(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("free space is only read on Linux and Windows")
	}
	if n := FreeSpace(t.TempDir()); n <= 0 {
		t.Fatalf("free space %d", n)
	}
}

func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges here")
		}
		t.Fatal(err)
	}
}

func TestImportRefusesTargetFolderLinkedOutside(t *testing.T) {
	w := setup(t)
	outside := t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(w.lib, "Dune - Part Two (2024)"))
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Result[0].Outcome != "refused" || !strings.Contains(strings.Join(imp.Plan[0].Why, " "), "outside the Library folder") {
		t.Fatalf("import %+v %+v", imp.Plan, imp.Result)
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("written outside: %v", ents)
	}
}

func TestHoldPathRefusesHoldingFolderLinkedOutside(t *testing.T) {
	lib, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(lib, "M", "a.mkv"), 10)
	symlinkOrSkip(t, outside, filepath.Join(lib, HoldingDir))
	if _, err := holdPath(lib, "M/a.mkv", time.Now()); err == nil {
		t.Fatal("a holding folder outside the Library folder was accepted")
	}
}

func TestReplaceWithNewNameSurvivesCrashAfterHold(t *testing.T) {
	w, _, oldID := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		"Dune.Part.Two.2024.720p.HDTV.x264-OTHER")
	ctx := context.Background()
	crashAt = func(s string) error {
		if s == "held_late" {
			return os.ErrDeadlineExceeded
		}
		return nil
	}
	_, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	crashAt = nil
	if err == nil {
		t.Fatal("no crash")
	}
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("restart %+v %v", imp, err)
	}
	held, _ := filepath.Glob(filepath.Join(w.lib, HoldingDir, "*", "Dune - Part Two (2024)", "*720p*"))
	if len(held) != 1 {
		t.Fatalf("held %v", held)
	}
	var old mediafiles.File
	_ = database.Get(ctx, w.s.Read(), "files", oldID, &old)
	rel, _ := filepath.Rel(w.lib, held[0])
	if old.State != "replaced" || old.RelativePath != filepath.ToSlash(rel) {
		t.Fatalf("old %s %q, held at %s", old.State, old.RelativePath, rel)
	}
}
