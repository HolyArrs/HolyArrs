package migrate

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// TestMoveResumedFourKCopyComesOver: a move that made the title and
// stopped before its 4K copy (a restart) adds the copy when it resumes,
// since the title is its own; a later move says to add it by hand.
func TestMoveResumedFourKCopyComesOver(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	ctx := context.Background()
	var id, fourK int64
	if err := l.st.Read().QueryRow("SELECT title_id, id FROM versions WHERE four_k = 1 AND is_main = 0").Scan(&id, &fourK); err != nil {
		t.Fatal(err)
	}
	// As if the move had stopped right after the title's mark: no 4K
	// Version yet, and the title as the 1080p app had it.
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{"DELETE FROM files WHERE version_id = ?", "DELETE FROM versions WHERE id = ?"} {
			if _, err := tx.Exec(q, fourK); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE versions SET keep_up_to_date = 1 WHERE title_id = ? AND is_main = 1", id); err != nil {
			return err
		}
		return SetKeepUpToDateTx(tx, id, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	it := item(t, l.plan(t), GroupTitles, "title:movie:tmdb:603")
	by := domain.Actor{Kind: "user", ID: 1}
	later := &run{m: l.mv, ctx: ctx, move: mv.ID + 1000, by: by}
	if d := later.finishMovedTitle(id, it.Key, it.Title); !strings.Contains(d, "add its 4K copy on the title page") {
		t.Fatalf("a later move's line %q", d)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 0 {
		t.Fatalf("a later move added %d 4K Versions", n)
	}
	same := &run{m: l.mv, ctx: ctx, move: mv.ID, by: by}
	if d := same.finishMovedTitle(id, it.Key, it.Title); strings.Contains(d, "add its 4K copy") || !strings.Contains(d, "its 4K copy came over as a 4K Version") {
		t.Fatalf("the resumed move's line %q", d)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1 AND keep_up_to_date = 1"); n != 1 {
		t.Fatalf("%d 4K Versions kept up to date after resuming, want 1", n)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1"); n != 1 {
		t.Fatal("the resumed title is not kept up to date for its 4K copy")
	}
}

// TestMoveWantedFourKCopyFolderEverySystemMakes: a 4K copy not downloaded
// yet has no folder, so its folder must take a name every system can
// create, as for any new Version; the dry run and the real run agree it is
// not brought. A folder already on disk keeps the name its app gave it.
func TestMoveWantedFourKCopyFolderEverySystemMakes(t *testing.T) {
	l, _, _ := fourKLabIn(t, false, "", "", "The Matrix: 4K (1999)")
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	// Windows refuses the colon folder one step earlier, at planning.
	windowsLine := "its 4K copy is not placed: its folder name " + strconv.Quote("The Matrix: 4K (1999)") + " cannot be a Version's folder"
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "its 4K copy will not be brought: "+library.ErrVersionFolder.Error()) && !(runtime.GOOS == "windows" && strings.Contains(dl.Detail, windowsLine)) {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 0 {
		t.Fatalf("%d 4K Versions with a folder name some systems cannot make", n)
	}
}

// TestMoveMainCopyInItsOwnFolderIgnoresTheFourKName: the 1080p app's file
// is gone and the 4K copy lives in a folder of its own, so a file in the
// main copy's folder that happens to share the 4K file's name is still the
// main copy's: it is picked as adopting always picks, the biggest movie
// video, not a smaller one.
func TestMoveMainCopyInItsOwnFolderIgnoresTheFourKName(t *testing.T) {
	l, _, _ := fourKLabNamed(t, false, "", "Feature.mkv")
	dir := filepath.Dir(l.file)
	if err := os.Remove(l.file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Feature.mkv"), []byte(strings.Repeat("x", 100)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Alternate.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	var rel string
	if err := l.st.Read().QueryRow("SELECT f.relative_path FROM files f JOIN versions v ON v.id = f.version_id WHERE v.is_main = 1").Scan(&rel); err != nil {
		t.Fatalf("%v: %+v", err, lineOf(t, mv.Report, "title:movie:tmdb:603"))
	}
	if filepath.Base(filepath.FromSlash(rel)) != "Feature.mkv" {
		t.Fatalf("the main copy took %s", rel)
	}
}

// TestMoveFourKCopyInTheSameFolderByAnotherPath: the 4K app's folder is
// another path to the main copy's folder (a link here; a name in another
// case where the file system ignores case), so the two copies would share
// one folder; the 4K copy is not brought, as for the same name.
func TestMoveFourKCopyInTheSameFolderByAnotherPath(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	fourKDir := filepath.Dir(fourKFile)
	if err := os.RemoveAll(fourKDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(l.file), fourKDir); err != nil {
		t.Skipf("no links here: %v", err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "a 4K Version needs a folder of its own") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 0 {
		t.Fatalf("%d 4K Versions on the main copy's own folder", n)
	}
}

// TestMoveResumedFourKCopyOnTheMainFolderByAnotherPath: the 4K app's
// folder is a link to the main copy's folder, so the move brings no 4K
// copy; resuming the same move does not bring it either.
func TestMoveResumedFourKCopyOnTheMainFolderByAnotherPath(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	fourKDir := filepath.Dir(fourKFile)
	if err := os.RemoveAll(fourKDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(l.file), fourKDir); err != nil {
		t.Skipf("no links here: %v", err)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	var id int64
	if err := l.st.Read().QueryRow("SELECT id FROM titles WHERE state != 'removed'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	it := item(t, l.plan(t), GroupTitles, "title:movie:tmdb:603")
	same := &run{m: l.mv, ctx: context.Background(), move: mv.ID, by: domain.Actor{Kind: "user", ID: 1}}
	if d := same.finishMovedTitle(id, it.Key, it.Title); !strings.Contains(d, "a 4K Version needs a folder of its own") {
		t.Fatalf("the resumed move's line %q", d)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 0 {
		t.Fatalf("%d 4K Versions on the main copy's own folder after resuming", n)
	}
}

// TestMoveResumedFourKCopyFinishesItsSteps: the move stopped after adding
// the 4K Version but before linking its file and keeping it up to date;
// resuming the same move does both.
func TestMoveResumedFourKCopyFinishesItsSteps(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	ctx := context.Background()
	var id, fourK int64
	if err := l.st.Read().QueryRow("SELECT title_id, id FROM versions WHERE four_k = 1 AND is_main = 0").Scan(&id, &fourK); err != nil {
		t.Fatal(err)
	}
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{"DELETE FROM files WHERE version_id = ?", "UPDATE items SET keep_up_to_date = 0 WHERE title_id = (SELECT title_id FROM versions WHERE id = ?)",
			"UPDATE versions SET keep_up_to_date = 1 WHERE is_main = 1 AND title_id = (SELECT title_id FROM versions WHERE id = ?)"} {
			if _, err := tx.Exec(q, fourK); err != nil {
				return err
			}
		}
		return SetKeepUpToDateTx(tx, id, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	it := item(t, l.plan(t), GroupTitles, "title:movie:tmdb:603")
	same := &run{m: l.mv, ctx: ctx, move: mv.ID, by: domain.Actor{Kind: "user", ID: 1}}
	if d := same.finishMovedTitle(id, it.Key, it.Title); !strings.Contains(d, "with The Matrix (1999)/The.Matrix.1999.2160p.mkv") {
		t.Fatalf("the resumed move's line %q", d)
	}
	for q, want := range map[string]int{
		"SELECT count(*) FROM files f JOIN versions v ON v.id = f.version_id WHERE v.four_k = 1 AND f.state = 'present'": 1,
		"SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1":                                   1,
		"SELECT count(*) FROM items WHERE keep_up_to_date = 1":                                                           1,
		"SELECT count(*) FROM versions WHERE is_main = 1 AND keep_up_to_date = 1":                                        0,
	} {
		if n := l.count(t, q); n != want {
			t.Fatalf("%s: %d, want %d", q, n, want)
		}
	}
}
