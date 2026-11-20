package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
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
