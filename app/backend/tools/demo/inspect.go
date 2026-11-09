package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// inspectTables are counted by the inspect command. Secret tables (users,
// sessions, api_keys, secrets) are counted, never read.
var inspectTables = []string{"titles", "parts", "items", "versions", "slots", "files", "releases", "downloads", "imports",
	"sources", "downloaders", "library_folders", "jobs", "health_checks", "asks", "users", "sessions", "api_keys", "secrets", "media_servers", "lookup_cache"}

// inspect prints table counts, the schema versions and a few states, read
// only, so the database is never changed.
func inspect(ctx context.Context, dir string, out io.Writer) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, database.MainFile))+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var v int
	if err := db.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&v); err != nil {
		return err
	}
	mainV, _ := database.SchemaVersions()
	fmt.Fprintf(out, "schema: database %d, this program %d\n", v, mainV)
	for _, t := range inspectTables {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			fmt.Fprintf(out, "%-16s ?\n", t)
			continue
		}
		fmt.Fprintf(out, "%-16s %d\n", t, n)
	}
	for _, q := range []string{
		"SELECT kind || ' ' || state, count(*) FROM titles GROUP BY 1",
		"SELECT state, count(*) FROM slots GROUP BY 1",
		"SELECT t.kind || ' ' || s.state, count(*) FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id GROUP BY 1",
		"SELECT t.name || ' ' || s.state, count(*) FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE t.kind = 'show' GROUP BY 1",
		"SELECT state || ' ' || COALESCE(state_reason, ''), count(*) FROM downloads GROUP BY 1",
		"SELECT state, count(*) FROM imports GROUP BY 1",
		"SELECT state, count(*) FROM releases GROUP BY 1",
		"SELECT kind || ' ' || state, count(*) FROM jobs GROUP BY 1",
		"SELECT kind || ' ' || path || ' ' || health, 1 FROM library_folders",
		"SELECT name || ' ' || state || ' ' || url, 1 FROM sources",
		"SELECT name || ' ' || state || ' ' || url || ' ' || last_error, 1 FROM downloaders",
		"SELECT check_key || ' ' || state || ' ' || message, 1 FROM health_checks",
		"SELECT grp || '.' || key, 1 FROM settings WHERE grp != 'links' ORDER BY 1",
	} {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			fmt.Fprintf(out, "%s: %v\n", q, err)
			continue
		}
		fmt.Fprintf(out, "-- %s\n", q)
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				fmt.Fprintf(out, "   %s: %d\n", k, n)
			}
		}
		rows.Close()
	}
	ev, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, database.EventsFile))+"?mode=ro")
	if err != nil {
		return err
	}
	defer ev.Close()
	rows, err := ev.QueryContext(ctx, "SELECT kind, count(*), max(summary) FROM events GROUP BY kind ORDER BY 2 DESC")
	if err != nil {
		return err
	}
	defer rows.Close()
	fmt.Fprintln(out, "-- events by kind (count, one summary)")
	for rows.Next() {
		var k, sum string
		var n int
		if rows.Scan(&k, &n, &sum) == nil {
			fmt.Fprintf(out, "   %-28s %5d  %s\n", k, n, sum)
		}
	}
	return rows.Err()
}

// queryTables are the tables query may read: the Library, searches,
// downloads, imports and Jobs. Settings, sign-in tables, Sources and
// Downloaders (their addresses may carry keys) and the schema never are.
var queryTables = map[string]bool{"titles": true, "parts": true, "items": true, "pressings": true, "versions": true, "slots": true,
	"files": true, "library_folders": true, "releases": true, "downloads": true, "imports": true, "jobs": true, "pick_rules": true,
	"rename_steps": true, "health_checks": true, "never_add": true}

// query prints the rows of one read-only query on the main database; it
// may name only the tables in queryTables.
func query(ctx context.Context, dir, q string, out io.Writer) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, database.MainFile))+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	// Every word of the query that names a table must name a listed one.
	// SQLite reads a table only by spelling its name, so a word check
	// covers quotes, brackets, sub-queries and table functions.
	names := map[string]bool{"sqlite_master": true, "sqlite_schema": true, "sqlite_temp_master": true, "sqlite_sequence": true}
	tables, err := db.QueryContext(ctx, "SELECT lower(name) FROM sqlite_master WHERE type IN ('table', 'view')")
	if err != nil {
		return err
	}
	for tables.Next() {
		var n string
		if tables.Scan(&n) == nil {
			names[n] = true
		}
	}
	tables.Close()
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, w := range words {
		if (names[w] || strings.HasPrefix(w, "pragma") || w == "attach") && !queryTables[w] {
			return fmt.Errorf("query: %s is not read here", w)
		}
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = fmt.Sprint(v)
		}
		fmt.Fprintln(out, strings.Join(parts, " | "))
	}
	return rows.Err()
}

// fingerprintTables hold sign-in data; fingerprint prints one hash per row,
// never a value, so a before and after can be compared.
var fingerprintTables = []string{"users", "sessions", "api_keys", "secrets", "backup_codes", "invites", "user_links"}

// fingerprint prints "table id hash" for every row of the sign-in tables
// and of the settings, read only.
func fingerprint(ctx context.Context, dir string, out io.Writer) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, database.MainFile))+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	for _, t := range append(fingerprintTables, "settings") {
		rows, err := db.QueryContext(ctx, "SELECT * FROM "+t+" ORDER BY id")
		if err != nil {
			fmt.Fprintf(out, "%s ? %v\n", t, err)
			continue
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		n := 0
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			h := sha256.New()
			var id any
			key := ""
			for i, v := range vals {
				fmt.Fprintf(h, "%s=%v;", cols[i], v)
				switch cols[i] {
				case "id":
					id = v
				case "key":
					key = fmt.Sprint(v)
				}
			}
			if t == "settings" && key == manifestKey {
				continue // the demo seed record is expected to change
			}
			fmt.Fprintf(out, "%s %v %x\n", t, id, h.Sum(nil)[:12])
			n++
		}
		rows.Close()
		fmt.Fprintf(out, "%s rows %d\n", t, n)
	}
	return nil
}
