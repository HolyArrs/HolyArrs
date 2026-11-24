package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A movie whose file is deleted from disk keeps Keep up to date while the
// switch is off (the old apps' default), and loses it once the owner turns
// the switch on, as Radarr's "unmonitor deleted movies" does.
func TestRescanUnkeepsDeletedMovieWhenAsked(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System)
	if err != nil || res[1].TitleID == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	heat := res[1].TitleID
	keep := func() (title, items, versions bool) {
		_ = s.Read().QueryRow("SELECT keep_up_to_date FROM titles WHERE id = ?", heat).Scan(&title)
		_ = s.Read().QueryRow("SELECT min(keep_up_to_date) FROM items WHERE title_id = ?", heat).Scan(&items)
		_ = s.Read().QueryRow("SELECT min(keep_up_to_date) FROM versions WHERE title_id = ?", heat).Scan(&versions)
		return
	}
	file := filepath.Join(lf.Path, "Heat (1995) {tmdb-949}", "heat.avi")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if r, err := svc.Rescan(ctx, domain.System); err != nil || r.Missing != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if ti, it, ve := keep(); !ti || !it || !ve {
		t.Fatalf("switch off: Keep up to date changed (%v %v %v)", ti, it, ve)
	}
	writeFile(t, file, "h")
	if r, _ := svc.Rescan(ctx, domain.System); r.Back != 1 {
		t.Fatalf("back %+v", r)
	}
	rules := importer.DefaultFileRules()
	rules.UnkeepDeletedMovies = true
	if err := importer.SaveFileRules(ctx, s, rules, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if r, err := svc.Rescan(ctx, domain.System); err != nil || r.Missing != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if ti, it, ve := keep(); ti || it || ve {
		t.Fatalf("switch on: Keep up to date still on (%v %v %v)", ti, it, ve)
	}
	var wanted int
	_ = s.Read().QueryRow("SELECT count(*) FROM slots WHERE state IN ('wanted', 'better_wanted') AND item_id IN (SELECT id FROM items WHERE title_id = ?)", heat).Scan(&wanted)
	if wanted != 0 {
		t.Fatalf("%d Slots still look for a copy", wanted)
	}
	es, _, _ := s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventTitleEdited}})
	if len(es) == 0 || es[0].Summary != "Heat: Keep up to date off, its file was deleted from disk" {
		t.Fatalf("events %+v", es)
	}
	// Dune's file is still there: it keeps its switch.
	var dune bool
	_ = s.Read().QueryRow("SELECT keep_up_to_date FROM titles WHERE id = ?", res[0].TitleID).Scan(&dune)
	if !dune {
		t.Fatal("the other movie lost Keep up to date")
	}
}

// A movie with two Versions whose one file goes: only that Version stops
// being kept up to date; the title and the Version still on disk keep it.
// When the last file goes too, the whole movie is switched off.
func TestUnkeepDeletedKeepsOtherVersion(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	ctx := context.Background()
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System)
	if err != nil || res[0].TitleID == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	heat := res[0].TitleID
	v4k, err := AddVersion(ctx, s, heat, VersionInput{Label: "4K", PickRulesID: fourKRules(t, s), LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var hdFile, slot4k int64
	_ = s.Read().QueryRow("SELECT s.file_id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND s.file_id IS NOT NULL", heat).Scan(&hdFile)
	_ = s.Read().QueryRow("SELECT id FROM slots WHERE version_id = ?", v4k.ID).Scan(&slot4k)
	at := database.FormatTime(now)
	var f4k int64
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		r, err := tx.Exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by)
		VALUES (?, 'system', ?, 'system', json_array(?), ?, 'Heat 4K/heat.mkv', ?, 'adopted', 'none')`, at, at, slot4k, v4k.ID, at)
		if err != nil {
			return err
		}
		f4k, _ = r.LastInsertId()
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", f4k, slot4k)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	unkeep := func(file int64) mediafiles.UnkeptDeleted {
		var u mediafiles.UnkeptDeleted
		if err := s.Write(ctx, func(tx *sql.Tx) error {
			var err error
			if _, err = tx.Exec("UPDATE files SET state = 'missing' WHERE id = ?", file); err == nil {
				u, _, err = mediafiles.UnkeepDeletedTx(ctx, tx, file, now, domain.System)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return u
	}
	keep := func(q string, arg int64) (on bool) {
		_ = s.Read().QueryRow(q, arg).Scan(&on)
		return
	}
	title, version := "SELECT keep_up_to_date FROM titles WHERE id = ?", "SELECT keep_up_to_date FROM versions WHERE id = ?"
	if u := unkeep(hdFile); !u.Version {
		t.Fatalf("first file: %+v, want only its Version", u)
	}
	if !keep(title, heat) || !keep(version, v4k.ID) {
		t.Fatal("the title or the 4K Version lost Keep up to date while the 4K file is on disk")
	}
	if keep("SELECT keep_up_to_date FROM versions WHERE title_id = ? AND id != (SELECT max(id) FROM versions)", heat) {
		t.Fatal("the Version whose file went is still kept up to date")
	}
	if u := unkeep(f4k); u.Version || keep(title, heat) || keep(version, v4k.ID) {
		t.Fatalf("last file: %+v, want the whole movie off", u)
	}
}
