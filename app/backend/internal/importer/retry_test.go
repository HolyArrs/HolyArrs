package importer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCopyRefusesTargetThatAppearsBeforeFinish(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "out", "dst.mkv")
	write(t, src, 100)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	beforeFinish = func() { write(t, dst, 7) }
	t.Cleanup(func() { beforeFinish = nil })
	if err := copyFile(src, dst); err == nil {
		t.Fatal("copy should refuse")
	}
	if fi, _ := os.Stat(dst); fi == nil || fi.Size() != 7 {
		t.Fatal("the file that appeared was overwritten")
	}
	if e, _ := os.ReadDir(filepath.Dir(dst)); len(e) != 1 {
		t.Fatalf("temp file left behind: %v", e)
	}
}

func TestLeftoverPartFileDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "dst.mkv")
	write(t, src, 100)
	write(t, dst+".holyarrs-part", 3)
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dst); fi == nil || fi.Size() != 100 {
		t.Fatal("copy wrong")
	}
}

func TestFolderNameFromVersionIsMadeSafe(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	_ = w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE versions SET folder_name = '../x' WHERE id = ?", w.vID)
		return err
	})
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !under(w.lib, imp.Plan[0].TargetPath) || imp.Plan[0].Method != "hardlink" {
		t.Fatalf("plan %+v", imp.Plan[0])
	}
}

func TestMapPathMatchesWholeSegments(t *testing.T) {
	maps := []domain.PathMap{{Theirs: "/data", Ours: "/mnt/data"}}
	if got := mapPath("/data2/x", maps); got != "/data2/x" {
		t.Fatalf("got %q", got)
	}
	if got := mapPath("/data/x", maps); got != "/mnt/data/x" {
		t.Fatalf("got %q", got)
	}
	if got := mapPath("/data", maps); got != "/mnt/data" {
		t.Fatalf("got %q", got)
	}
	if got := mapPath("/data/x", []domain.PathMap{{Theirs: "/data/", Ours: "/mnt/data/"}}); got != "/mnt/data/x" {
		t.Fatalf("got %q", got)
	}
}

func count(t *testing.T, s *database.Store, table string) int {
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func crashThenRetry(t *testing.T, first, second Ops) {
	w := setup(t)
	ctx := context.Background()
	func() {
		defer func() { _ = recover() }()
		_, _ = (&Importer{Store: w.s, Ops: first}).Import(ctx, w.dlID, false, domain.System)
		t.Fatal("first run should have crashed")
	}()
	imp, err := (&Importer{Store: w.s, Ops: second}).Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State != "done" || count(t, w.s, "imports") != 1 || count(t, w.s, "files") != 1 {
		t.Fatalf("import %+v, imports %d, files %d", imp, count(t, w.s, "imports"), count(t, w.s, "files"))
	}
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	if sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s", sl.State)
	}
}

func TestRetryAfterCrashFinishesHardlink(t *testing.T) {
	crashThenRetry(t, Ops{Link: func(a, b string) error {
		if err := os.Link(a, b); err != nil {
			return err
		}
		panic("crash after placing")
	}}, Ops{})
}

func TestRetryAfterCrashFinishesCopy(t *testing.T) {
	exdev := func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} }
	crashThenRetry(t, Ops{Link: exdev, Copy: func(a, b string) error {
		if err := copyFile(a, b); err != nil {
			return err
		}
		panic("crash after placing")
	}}, Ops{Link: exdev})
}

func TestRetryRefusesDifferentFileAtTarget(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
	write(t, src, 100)
	if err := os.WriteFile(dst, bytes.Repeat([]byte{1}, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if sameContent(src, dst, 100) {
		t.Fatal("different content seen as same")
	}
}

func TestSlotAndImportStayConsistentOnFailure(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TRIGGER no_lib BEFORE UPDATE OF state ON slots WHEN NEW.state = 'in_library' BEGIN SELECT RAISE(ABORT, 'boom'); END")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err == nil {
		t.Fatal("import should fail")
	}
	var sl mediafiles.Slot
	_ = database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl)
	imps, _ := database.Select[mediafiles.Import](ctx, w.s.Read(), "imports", "")
	if sl.State != mediafiles.SlotImporting || sl.FileID != nil || len(imps) != 1 || imps[0].State != "placing" || count(t, w.s, "files") != 0 {
		t.Fatalf("slot %+v imports %+v", sl, imps)
	}
}

func TestCopyFallsBackWhenLinkUnsupported(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "out", "dst.mkv")
	write(t, src, 100)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	linkHook = func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EPERM} }
	t.Cleanup(func() { linkHook = nil })
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dst); fi == nil || fi.Size() != 100 {
		t.Fatal("copy wrong")
	}
	if e, _ := os.ReadDir(filepath.Dir(dst)); len(e) != 1 {
		t.Fatalf("temp file left behind: %v", e)
	}
	dst2 := filepath.Join(dir, "out", "dst2.mkv")
	write(t, dst2, 7)
	if err := copyFile(src, dst2); err == nil {
		t.Fatal("copy should refuse an existing target")
	}
	if fi, _ := os.Stat(dst2); fi.Size() != 7 {
		t.Fatal("existing target was overwritten")
	}
}

// An identical file is taken whatever its age; another app may have placed it.
func TestRetryTakesIdenticalTargetOlderThanImport(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
	write(t, src, 100)
	if err := os.WriteFile(dst, mustRead(t, src), 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	old := started.Add(-time.Hour)
	_ = os.Chtimes(dst, old, old)
	step := mediafiles.PlanStep{SourceFile: src, TargetPath: dst, Method: "hardlink"}
	if r := place(step, Ops{}, started); r.Outcome != "placed" {
		t.Fatalf("older identical file not adopted: %+v", r)
	}
	fresh := started.Add(-time.Second)
	_ = os.Chtimes(dst, fresh, fresh)
	if r := place(step, Ops{}, started); r.Outcome != "placed" {
		t.Fatalf("file within slack not adopted: %+v", r)
	}
}

func mustRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSweepRemovesOnlyStalePartFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, ".holyarrs-123.part")
	live := filepath.Join(dir, ".holyarrs-456.part")
	other := filepath.Join(dir, "movie.mkv")
	lookalike := filepath.Join(dir, "notes.holyarrs-1.part.txt")
	for _, p := range []string{stale, live, other, lookalike} {
		write(t, p, 10)
	}
	activeParts.Store(live, true)
	t.Cleanup(func() { activeParts.Delete(live) })
	sweepStaleParts(dir)
	for p, want := range map[string]bool{stale: false, live: true, other: true, lookalike: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Fatalf("%s: exists=%v want %v", filepath.Base(p), err == nil, want)
		}
	}
}

func TestImportRemovesStalePartFromTargetFolder(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	folder := filepath.Join(w.lib, FolderName("Dune: Part Two", 2024))
	stale := filepath.Join(folder, ".holyarrs-999.part")
	keep := filepath.Join(folder, "poster.jpg")
	write(t, stale, 50)
	write(t, keep, 5)
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("stale part file was not removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("an unrelated file was touched")
	}
}

func TestImportThroughWindowsMapping(t *testing.T) {
	w := setup(t)
	parent, base := filepath.Dir(w.dl), filepath.Base(w.dl)
	maps, _ := json.Marshal([]domain.PathMap{{Theirs: `D:\Downloads`, Ours: parent}})
	mapExec(t, w.s, "UPDATE downloaders SET path_map = ?", string(maps))
	mapExec(t, w.s, "UPDATE downloads SET save_path = ?", `d:\downloads\`+base)
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %v %+v", err, imp)
	}
}

func TestImportRefusesDotDotFolder(t *testing.T) {
	w := setup(t)
	mapExec(t, w.s, "UPDATE downloads SET save_path = ?", filepath.Join(w.dl, "x")+string(filepath.Separator)+".."+string(filepath.Separator)+filepath.Base(w.dl))
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if imp.State == "done" || len(imp.Plan) != 1 || imp.Plan[0].Method != "refuse" || !strings.Contains(strings.Join(imp.Plan[0].Why, " "), `".."`) {
		t.Fatalf("import %s plan %+v", imp.State, imp.Plan)
	}
	if n := count(t, w.s, "files"); n != 0 {
		t.Fatalf("%d files placed", n)
	}
}

func mapExec(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
