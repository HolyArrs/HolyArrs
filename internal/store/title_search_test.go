package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

func TestTitleSearchMigrationAndIndexMaintenance(t *testing.T) {
	src, err := fs.Sub(embedded, "migrations/main")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	before := fstest.MapFS{}
	for _, m := range ms {
		if m.version <= 38 {
			before[m.name] = &fstest.MapFile{Data: []byte(m.sql)}
		}
	}
	dir := t.TempDir()
	s, err := Open(dir, Options{MainMigrations: before})
	if err != nil {
		t.Fatal(err)
	}
	title := domain.Title{Kind: "movie", Name: "Amélie", SortName: "amelie", Aliases: []string{"Le Fabuleux Destin", "Fête\nNoire", "Light & Shadow"}, MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active"}
	title.NewPartsPolicy, title.ReleaseGate, title.GateDriver, title.Status = "all", "released", "none", "announced"
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := Insert(tx, "titles", &title, domain.System)
		return err
	})
	if err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	backups, err := filepath.Glob(filepath.Join(dir, "backups", "main-before-0039-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("migration backup: %v (%v)", backups, err)
	}
	match := func(q string, want int) {
		t.Helper()
		var n int
		if err := s.Read().QueryRow("SELECT count(*) FROM title_search WHERE title_search MATCH ?", q).Scan(&n); err != nil || n != want {
			t.Fatalf("match %q: %d, want %d (%v)", q, n, want, err)
		}
	}
	match("amelie", 1)
	match("fabuleux", 1)
	match("noire", 1)
	match("shadow", 1)
	match("u0026", 0)

	indexRows := func() []string {
		t.Helper()
		rows, err := s.Read().Query("SELECT printf('%d:%s', id, hex(block)) FROM title_search_data ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				t.Fatal(err)
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	indexBefore := indexRows()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		// Store.Update writes all columns even when only unrelated metadata changed.
		title.Status = "released"
		return Update(tx, "titles", &title, domain.System)
	}); err != nil {
		t.Fatal(err)
	}
	if after := indexRows(); !reflect.DeepEqual(indexBefore, after) {
		t.Fatal("unchanged search values rewrote the full-text index")
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		title.Name, title.SortName, title.Aliases = "Cidade de Deus", "cidade de deus", []string{"City of God"}
		return Update(tx, "titles", &title, domain.System)
	}); err != nil {
		t.Fatal(err)
	}
	match("amelie", 0)
	match("fabuleux", 0)
	match("cidade", 1)
	match("city", 1)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM titles WHERE id = ?", title.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	match("cidade", 0)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		title.ID = 0
		_, err := Insert(tx, "titles", &title, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	match("city", 1)
	// Soft removal and restoration change visibility without rewriting the index.
	indexBefore = indexRows()
	for _, state := range []string{"removed", "active"} {
		if err := s.Write(context.Background(), func(tx *sql.Tx) error {
			title.State = state
			return Update(tx, "titles", &title, domain.System)
		}); err != nil {
			t.Fatal(err)
		}
		found, err := s.SearchTitles(context.Background(), "city")
		if err != nil || len(found) != map[string]int{"removed": 0, "active": 1}[state] {
			t.Fatalf("search in state %s: %+v (%v)", state, found, err)
		}
	}
	if after := indexRows(); !reflect.DeepEqual(indexBefore, after) {
		t.Fatal("removing/restoring title rewrote the full-text index")
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM title_search")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	match("city", 0)
	if err := s.RebuildTitleSearch(context.Background()); err != nil {
		t.Fatal(err)
	}
	match("city", 1)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.RebuildTitleSearch(cancelled); err == nil {
		t.Fatal("cancelled rebuild succeeded")
	}
	match("city", 1)
}
