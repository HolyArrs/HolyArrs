package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// The Downloads view reads the newest Import per Download and the Downloads
// waiting to be cleared on every refresh; neither may read a whole table.
func TestDownloadsViewQueriesUseIndexes(t *testing.T) {
	db, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		"SELECT download_id, state FROM imports WHERE download_id IN (1, 2, 3) ORDER BY id",
		"SELECT id, clear_retry_at, clear_tries, clear_error FROM downloads WHERE clear_retry_at IS NOT NULL",
		"SELECT id, state FROM downloads WHERE clear_retry_at IS NOT NULL AND clear_retry_at <= '2026' ORDER BY clear_retry_at, id",
	} {
		rows, err := db.Read().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+q)
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
		rows.Close()
		got := strings.Join(plan, "; ")
		if strings.Contains(got, "SCAN imports") || strings.Contains(got, "SCAN downloads") {
			t.Errorf("%s\n  reads the whole table: %s", q, got)
		}
	}
}

// BenchmarkDownloadsViewQueries times one Downloads view's two lookups with
// 30,000 Downloads and Imports, with the 0112 indexes and without them.
func BenchmarkDownloadsViewQueries(b *testing.B) {
	db, err := Open(b.TempDir(), Options{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	err = db.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO downloaders(id,created_at,created_by,updated_at,updated_by,name,adapter,protocol,url) VALUES(1,'t','{}','t','{}','d','sabnzbd','usenet','http://bench')`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 30000)
			 INSERT INTO downloads(id,created_at,created_by,updated_at,updated_by,downloader_id,external_id,state,started_by,clear_retry_at)
			 SELECT i,'t','{}','t','{}',1,'x'||i,'cleaned','{}',CASE WHEN i % 1000 = 0 THEN '2026-01-01' END FROM n`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 30000)
			 INSERT INTO imports(created_at,created_by,updated_at,updated_by,download_id,from_path,kind,mode,state)
			 SELECT 't','{}','t','{}',i,'/p','movie','auto','done' FROM n`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	ids := make([]any, 200)
	for i := range ids {
		ids[i] = 29800 + i
	}
	view := func(b *testing.B) {
		for b.Loop() {
			for _, q := range []struct {
				sql  string
				args []any
			}{
				{"SELECT download_id, state FROM imports WHERE download_id IN (?" + strings.Repeat(", ?", len(ids)-1) + ") ORDER BY id", ids},
				{"SELECT id, clear_retry_at, clear_tries, clear_error FROM downloads WHERE clear_retry_at IS NOT NULL", nil},
			} {
				rows, err := db.Read().QueryContext(ctx, q.sql, q.args...)
				if err != nil {
					b.Fatal(err)
				}
				for rows.Next() {
				}
				rows.Close()
			}
		}
	}
	b.Run("indexed", view)
	err = db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DROP INDEX imports_download; DROP INDEX downloads_clear_retry")
		return err
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Run("no_index", view)
}
