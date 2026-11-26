package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// A deleted movie's held files are listed with where they came from, and a
// restore puts them back and brings the movie back to the Library.
func TestRestoreHeldBringsTheMovieBack(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	if _, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	held, err := svc.HeldFiles(ctx, 0)
	if err != nil || len(held) != 2 {
		t.Fatalf("held %+v %v", held, err)
	}
	h := held[0]
	if !h.TitleDeleted || h.TitleID != id || h.ExpiresAt == nil || h.ExpiresAt.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("held %+v", h)
	}
	// A taken place is never overwritten.
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(fs[1].RelativePath)), "someone")
	res, back, err := svc.RestoreHeld(ctx, []int64{fs[0].ID, fs[1].ID, 999}, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 || res[0].Result != "restored" || !strings.Contains(res[1].Result, "already") || res[2].Result != "not in a holding folder" {
		t.Fatalf("results %+v", res)
	}
	if len(back) != 1 || back[0] != id {
		t.Fatalf("back %v", back)
	}
	if !onDisk(lf, fs[0].RelativePath) {
		t.Fatal("not back on disk")
	}
	if f := fileRow(t, s, fs[0].ID); f.State != "present" || f.RelativePath != fs[0].RelativePath {
		t.Fatalf("row %+v", f)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.State != "active" {
		t.Fatalf("title %s", ti.State)
	}
	if sl := slotsOf(t, s, id); sl[0].State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s", sl[0].State)
	}
	if b, _ := os.ReadFile(filepath.Join(lf.Path, filepath.FromSlash(fs[1].RelativePath))); string(b) != "someone" {
		t.Fatal("overwrote a file")
	}
	if held, _ := svc.HeldFiles(ctx, 0); len(held) != 1 || held[0].ID != fs[1].ID {
		t.Fatalf("still held %+v", held)
	}
}

// a copy an upgrade replaced waits in the same holding folder; it is
// listed with what replaced it and can be restored, never over the new copy.
func TestRestoreHeldListsReplacedCopies(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	old, newer := fs[1], fs[0]
	day := svc.now().Format("2006-01-02")
	held := importer.HoldingDir + "/" + day + "/" + old.RelativePath
	if err := os.MkdirAll(filepath.Dir(filepath.Join(lf.Path, filepath.FromSlash(held))), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(lf.Path, filepath.FromSlash(old.RelativePath)), filepath.Join(lf.Path, filepath.FromSlash(held))); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ? WHERE id = ?", held, old.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.AddEvent(ctx, &database.Event{Kind: "file_replaced", Data: map[string]any{"old_file_id": old.ID, "new_file_id": newer.ID}})
	list, err := svc.HeldFiles(ctx, 0)
	if err != nil || len(list) != 1 || list[0].ID != old.ID || !list[0].Replaced || list[0].ReplacedBy != newer.RelativePath || list[0].Path != old.RelativePath {
		t.Fatalf("held %+v %v", list, err)
	}
	// Its spot taken by the new copy: said so, nothing overwritten.
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(old.RelativePath)), "new")
	res, _, err := svc.RestoreHeld(ctx, []int64{old.ID}, "", domain.System)
	if err != nil || len(res) != 1 || !strings.Contains(res[0].Result, "already") {
		t.Fatalf("taken %+v %v", res, err)
	}
	_ = os.Remove(filepath.Join(lf.Path, filepath.FromSlash(old.RelativePath)))
	// Its own name is free, but the better copy (another name) still holds
	// the place: asked, not quietly kept beside it.
	res, _, err = svc.RestoreHeld(ctx, []int64{old.ID}, "", domain.System)
	if err != nil || !res[0].Taken || !res[0].CanSwap || onDisk(lf, old.RelativePath) {
		t.Fatalf("asked %+v %v", res, err)
	}
	res, _, err = svc.RestoreHeld(ctx, []int64{old.ID}, RestoreBoth, domain.System)
	if err != nil || res[0].Result != "restored" || !onDisk(lf, old.RelativePath) || !onDisk(lf, newer.RelativePath) {
		t.Fatalf("restore %+v %v", res, err)
	}
	if f := fileRow(t, s, old.ID); f.State != "present" || f.RelativePath != old.RelativePath {
		t.Fatalf("row %+v", f)
	}
	_ = id
}

// The upgrade renamed the file (new quality in the name); Swap back
// sends the better copy to Recently deleted and the old copy returns under
// its own name. Upgraded twice: each copy comes back the same way.
func TestRestoreReplacedOtherNameSwapAndUpgradeTwice(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	old, newer := fs[1], fs[0]
	day := svc.now().Format("2006-01-02")
	hold := func(f int64, rel string) string {
		h := importer.HoldingDir + "/" + day + "/" + rel
		if err := os.MkdirAll(filepath.Dir(filepath.Join(lf.Path, filepath.FromSlash(h))), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(lf.Path, filepath.FromSlash(rel)), filepath.Join(lf.Path, filepath.FromSlash(h))); err != nil {
			t.Fatal(err)
		}
		if err := s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ? WHERE id = ?", h, f)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return h
	}
	hold(old.ID, old.RelativePath)
	_ = s.AddEvent(ctx, &database.Event{Kind: "file_replaced", Data: map[string]any{"old_file_id": old.ID, "new_file_id": newer.ID, "from": old.RelativePath}})
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE id IN (SELECT id FROM slots WHERE item_id IN (SELECT id FROM items WHERE title_id = ?) LIMIT 1)", newer.ID, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, _, err := svc.RestoreHeld(ctx, []int64{old.ID}, RestoreSwap, domain.System)
	if err != nil || res[0].Result != "restored" || !onDisk(lf, old.RelativePath) || onDisk(lf, newer.RelativePath) {
		t.Fatalf("swap %+v %v", res, err)
	}
	if f := fileRow(t, s, newer.ID); f.State != "replaced" {
		t.Fatalf("newer %+v", f)
	}
	var onOld int
	_ = s.Read().QueryRowContext(ctx, "SELECT count(*) FROM slots WHERE file_id = ?", old.ID).Scan(&onOld)
	if onOld == 0 {
		t.Fatal("slot did not follow")
	}
	// The better copy waits now and remembers its own name; swapping again
	// puts it back and sends the old one away: a second round trip works.
	l, _ := svc.HeldFiles(ctx, id)
	if len(l) != 1 || l[0].ID != newer.ID || l[0].Path != newer.RelativePath {
		t.Fatalf("held %+v", l)
	}
	res, _, err = svc.RestoreHeld(ctx, []int64{newer.ID}, RestoreSwap, domain.System)
	if err != nil || res[0].Result != "restored" || !onDisk(lf, newer.RelativePath) || onDisk(lf, old.RelativePath) {
		t.Fatalf("swap again %+v %v", res, err)
	}
}

// The badge never counts a day that is already emptied, even once the
// keep setting grows past it.
func TestHeldCountSkipsEmptiedDays(t *testing.T) {
	svc, s, lf, _, fs := deleteWorld(t)
	ctx := context.Background()
	h := importer.HoldingDir + "/" + svc.now().Format("2006-01-02") + "/" + fs[1].RelativePath
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'gone', relative_path = ? WHERE id = ?", h, fs[1].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.HeldCount(ctx, 0); err != nil || n != 0 {
		t.Fatalf("emptied day counted %d %v", n, err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(h)), "x")
	if n, _ := svc.HeldCount(ctx, 0); n != 1 {
		t.Fatalf("held day %d", n)
	}
}

// A replaced copy whose place the newer copy took comes back by a
// swap (the newer copy goes to Recently deleted, the Slot follows) or
// beside it under a free name; nothing is overwritten.
func TestRestoreReplacedSwapAndKeepBoth(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	old, newer := fs[1], fs[0]
	day := svc.now().Format("2006-01-02")
	held := importer.HoldingDir + "/" + day + "/" + newer.RelativePath + ".1"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(lf.Path, filepath.FromSlash(held))), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(lf.Path, filepath.FromSlash(old.RelativePath)), filepath.Join(lf.Path, filepath.FromSlash(held))); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ? WHERE id = ?", held, old.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The old copy had the same name as the newer one; its held name got a .1.
	_ = s.AddEvent(ctx, &database.Event{Kind: "file_replaced", Data: map[string]any{"old_file_id": old.ID, "new_file_id": newer.ID, "from": newer.RelativePath}})
	if n, err := svc.HeldCount(ctx, 0); err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	if n, _ := svc.HeldCount(ctx, id+1000); n != 0 {
		t.Fatalf("other title count %d", n)
	}
	if l, _ := svc.HeldFiles(ctx, id); len(l) != 1 || l[0].Path != newer.RelativePath {
		t.Fatalf("list %+v", l)
	}
	res, _, err := svc.RestoreHeld(ctx, []int64{old.ID}, "", domain.System)
	if err != nil || !res[0].Taken || !res[0].CanSwap {
		t.Fatalf("taken %+v %v", res, err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE id IN (SELECT id FROM slots WHERE item_id IN (SELECT id FROM items WHERE title_id = ?) LIMIT 1)", newer.ID, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, _, err = svc.RestoreHeld(ctx, []int64{old.ID}, RestoreSwap, domain.System)
	if err != nil || res[0].Result != "restored" {
		t.Fatalf("swap %+v %v", res, err)
	}
	if f := fileRow(t, s, old.ID); f.State != "present" || f.RelativePath != newer.RelativePath {
		t.Fatalf("old row %+v", f)
	}
	if f := fileRow(t, s, newer.ID); f.State != "replaced" || !strings.HasPrefix(f.RelativePath, importer.HoldingDir+"/") {
		t.Fatalf("newer row %+v", f)
	}
	var onOld int
	_ = s.Read().QueryRowContext(ctx, "SELECT count(*) FROM slots WHERE file_id = ?", old.ID).Scan(&onOld)
	if onOld == 0 {
		t.Fatal("slot did not follow the swap")
	}
	// The newer copy is now held and can come back beside it.
	res, _, err = svc.RestoreHeld(ctx, []int64{newer.ID}, RestoreBoth, domain.System)
	if err != nil || res[0].Result != "restored" || !strings.Contains(res[0].Path, " (2)") || !onDisk(lf, res[0].Path) || !onDisk(lf, newer.RelativePath) {
		t.Fatalf("both %+v %v", res, err)
	}
}

// A file of another title at the place is never swapped out (its
// title's slots would be rewired): only "keep both" is offered.
func TestRestoreNeverSwapsAnotherTitlesFile(t *testing.T) {
	svc, s, lf, _, fs := deleteWorld(t)
	ctx := context.Background()
	old, other := fs[1], fs[0]
	otherTitle, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	day := svc.now().Format("2006-01-02")
	held := importer.HoldingDir + "/" + day + "/" + other.RelativePath + ".1"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(lf.Path, filepath.FromSlash(held))), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(lf.Path, filepath.FromSlash(old.RelativePath)), filepath.Join(lf.Path, filepath.FromSlash(held))); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE files SET state = 'gone', relative_path = ? WHERE id = ?", held, old.ID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE versions SET library_folder_id = ? WHERE title_id = ?", lf.ID, otherTitle); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET version_id = (SELECT id FROM versions WHERE title_id = ? LIMIT 1) WHERE id = ?", otherTitle, other.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.AddEvent(ctx, &database.Event{Kind: domain.EventFileHeld, Data: map[string]any{"file_id": old.ID, "to": held, "from": other.RelativePath}})
	res, _, err := svc.RestoreHeld(ctx, []int64{old.ID}, RestoreSwap, domain.System)
	if err != nil || !res[0].Taken || res[0].CanSwap {
		t.Fatalf("swap of another title's file %+v %v", res, err)
	}
	if f := fileRow(t, s, other.ID); f.State != "present" || f.RelativePath != other.RelativePath {
		t.Fatalf("other title's file moved %+v", f)
	}
	res, _, err = svc.RestoreHeld(ctx, []int64{old.ID}, RestoreBoth, domain.System)
	if err != nil || res[0].Result != "restored" || !strings.Contains(res[0].Path, " (2)") {
		t.Fatalf("both %+v %v", res, err)
	}
}
