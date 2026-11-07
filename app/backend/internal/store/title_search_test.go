package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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
		// The old schema lacks later title columns, so write the row by hand.
		aliases, _ := json.Marshal(title.Aliases)
		res, err := tx.Exec(`INSERT INTO titles(created_at, created_by, updated_at, updated_by, kind, name, sort_name, aliases,
			metadata_source, new_parts_policy, release_gate, gate_driver, status, why_added, state)
			VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			title.Kind, title.Name, title.SortName, string(aliases),
			title.MetadataSource, title.NewPartsPolicy, title.ReleaseGate, title.GateDriver, title.Status, title.WhyAdded, title.State)
		if err == nil {
			title.ID, err = res.LastInsertId()
		}
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
	backups, err := filepath.Glob(filepath.Join(dir, "backups", "main-pre-update-*.db"))
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
	if err := Get(context.Background(), s.Read(), "titles", title.ID, &title); err != nil {
		t.Fatal(err)
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

func TestSearchTitlesOriginalNameAccentsAndYear(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	orig := "Le Fabuleux Destin d'Amélie Poulain"
	add := func(name string, year int, original *string) {
		t.Helper()
		title := domain.Title{Kind: "movie", Name: name, SortName: name, Year: &year, OriginalName: original, MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active",
			NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := Insert(tx, "titles", &title, domain.System); return err }); err != nil {
			t.Fatal(err)
		}
	}
	add("Amelie", 2001, &orig)
	add("Dune", 1984, nil)
	add("Dune", 2021, nil)
	names := func(q string) []string {
		t.Helper()
		ts, err := s.SearchTitles(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, x := range ts {
			out = append(out, fmt.Sprintf("%s %d", x.Name, *x.Year))
		}
		return out
	}
	for q, want := range map[string][]string{
		"fabuleux":  {"Amelie 2001"},
		"AMÉLIE":    {"Amelie 2001"},
		"poulain":   {"Amelie 2001"},
		"dune 2021": {"Dune 2021"},
		"DÜNE":      {"Dune 1984", "Dune 2021"},
		"dune 1999": {},
	} {
		if got := names(q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, want %q", q, got, want)
		}
	}
}
