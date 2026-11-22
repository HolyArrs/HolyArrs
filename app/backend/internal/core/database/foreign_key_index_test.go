package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// bigTables grow with the Library and the search history. A foreign key
// that points at one of them needs an index on its child column, or every
// delete of a parent row scans the whole child table once per row: a
// Source delete then took minutes.
var bigTables = map[string]bool{"sources": true, "releases": true, "downloads": true, "titles": true, "items": true,
	"parts": true, "versions": true, "slots": true, "files": true}

func TestForeignKeysToBigTablesHaveAnIndex(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.Read()
	type fk struct{ table, column, parent string }
	var fks []fk
	// The first column of each key; a key over several columns needs an
	// index that starts with it.
	rows, err := db.Query(`SELECT m.name, f."from", f."table" FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table' AND f.seq = 0 ORDER BY m.name, f."from"`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k fk
		if err := rows.Scan(&k.table, &k.column, &k.parent); err != nil {
			t.Fatal(err)
		}
		fks = append(fks, k)
	}
	_ = rows.Close()
	if len(fks) == 0 {
		t.Fatal("no foreign keys read")
	}
	for _, k := range fks {
		if !bigTables[k.parent] {
			continue
		}
		var n int
		// The column must be the first one of some index on the table, or
		// the table's INTEGER PRIMARY KEY, which is its rowid.
		err := db.QueryRow(`SELECT (SELECT count(*) FROM pragma_index_list(?1) l, pragma_index_info(l.name) i
			WHERE i.seqno = 0 AND i.name = ?2 AND l.partial = 0)
			+ (SELECT count(*) FROM pragma_table_info(?1) c WHERE c.name = ?2 AND c.pk = 1 AND upper(c.type) = 'INTEGER'
				AND (SELECT count(*) FROM pragma_table_info(?1) WHERE pk > 0) = 1)`, k.table, k.column).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("%s.%s points at %s but has no index; add CREATE INDEX %s_%s ON %s(%s)",
				k.table, k.column, k.parent, k.table, strings.TrimSuffix(k.column, "_id"), k.table, k.column)
		}
	}
}

// An exact outside-id lookup must use its index, not read every title.
func TestOutsideIDLookupsUseTheirIndex(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, k := range []string{"tmdb", "tvdb", "imdb", "tvmaze", "musicbrainz"} {
		rows, err := s.Read().Query("EXPLAIN QUERY PLAN SELECT id FROM titles WHERE kind = 'movie' AND state != 'removed' AND json_extract(source_ids, '$."+k+"') = ?", "1")
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		_ = rows.Close()
		if got := strings.Join(plan, "; "); !strings.Contains(got, "INDEX titles_"+k) {
			t.Errorf("%s lookup plan: %s", k, got)
		}
	}
}

// A title saved by an older build with ids that are not valid JSON must not
// stop the index migration, or the whole update and the start with it.
func TestIndexMigrationTakesTitlesWithBadIDs(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: upTo(t, 132)})
	if err != nil {
		t.Fatal(err)
	}
	meta := `'t', '{}', 't', '{}'`
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, status, state, gate_driver)
				VALUES (1, ` + meta + `, 'movie', 'Bad', 'bad', '', 'local', 'by_hand', 'released', 'removed', 'none')`,
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, status, state, gate_driver)
				VALUES (2, ` + meta + `, 'movie', 'Good', 'good', '{"tmdb":"603"}', 'local', 'by_hand', 'released', 'active', 'none')`,
			// JSON5 SQLite reads already: its ids must survive.
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, status, state, gate_driver)
				VALUES (3, ` + meta + `, 'movie', 'Loose', 'loose', '{tmdb:''604'', imdb:''tt0133093'',}', 'local', 'by_hand', 'released', 'active', 'none')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("the migration refused a title with bad ids: %v", err)
	}
	defer s.Close()
	var bad, id string
	if err := s.Read().QueryRow("SELECT source_ids FROM titles WHERE id = 1").Scan(&bad); err != nil || bad != "{}" {
		t.Fatalf("bad ids now %q %v", bad, err)
	}
	if err := s.Read().QueryRow("SELECT id FROM titles WHERE json_extract(source_ids, '$.tmdb') = '603'").Scan(&id); err != nil || id != "2" {
		t.Fatalf("good title lookup %q %v", id, err)
	}
	var loose string
	if err := s.Read().QueryRow("SELECT source_ids FROM titles WHERE id = 3").Scan(&loose); err != nil || loose != `{"tmdb":"604","imdb":"tt0133093"}` {
		t.Fatalf("JSON5 ids now %q %v", loose, err)
	}
	if err := s.Read().QueryRow("SELECT id FROM titles WHERE json_extract(source_ids, '$.tmdb') = '604'").Scan(&id); err != nil || id != "3" {
		t.Fatalf("JSON5 title lookup %q %v", id, err)
	}
}
