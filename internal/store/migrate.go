package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	sql     string
	rebuild bool
}

func loadMigrations(src fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		num, _, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("migration %q must start with a positive number and an underscore", e.Name())
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrations %q and %q share number %d", prev, e.Name(), v)
		}
		seen[v] = e.Name()
		b, err := fs.ReadFile(src, e.Name())
		if err != nil {
			return nil, err
		}
		sql := string(b)
		out = append(out, migration{version: v, name: e.Name(), sql: sql,
			rebuild: strings.HasPrefix(sql, "-- holyarrs:foreign-key-rebuild\n") || strings.HasPrefix(sql, "-- holyarrs:foreign-key-rebuild\r\n")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func version(db *sql.DB) (int, error) {
	var v sql.NullInt64
	if err := db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&v); err != nil {
		return 0, err
	}
	return int(v.Int64), nil
}

// migrate applies pending migrations in order. Before each one it copies the
// database with VACUUM INTO, unless nothing has been applied yet.
func migrate(w *writer, src fs.FS, backupDir, label string) error {
	ms, err := loadMigrations(src)
	if err != nil {
		return err
	}
	err = w.doUncapped(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(
			version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`)
		return err
	})
	if err != nil {
		return err
	}
	cur, err := version(w.db)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.version <= cur {
			continue
		}
		if cur > 0 {
			if err := os.MkdirAll(backupDir, 0o750); err != nil {
				return err
			}
			name := fmt.Sprintf("%s-before-%04d-%s.db", label, m.version, time.Now().UTC().Format("20060102T150405.000Z"))
			if _, err := w.db.Exec("VACUUM INTO ?", filepath.Join(backupDir, name)); err != nil {
				return fmt.Errorf("backup before migration %s: %w", m.name, err)
			}
		}
		apply := w.doUncapped
		if m.rebuild {
			apply = w.doRebuild
		}
		err := apply(context.Background(), func(tx *sql.Tx) error {
			if _, err := tx.Exec(m.sql); err != nil {
				return err
			}
			_, err := tx.Exec("INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)",
				m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano))
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s (%s): %w", m.name, label, err)
		}
		cur = m.version
	}
	return nil
}
