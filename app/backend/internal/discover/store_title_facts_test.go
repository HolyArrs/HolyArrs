package discover

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The Library list size counts only the copies a title still has: a copy that
// was replaced or deleted stays as a row but is not summed.
func TestTitleFactsSizeSkipsReplaced(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ts, by := database.FormatTime(time.Now().UTC()), `{"kind":"system"}`
	var title int64
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		lf := ex(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, ?, ?, ?, ?, 'movie', 'Movies', 'ok', '[]')`, ts, by, ts, by, t.TempDir())
		var rules int64
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		title = ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
			VALUES (?, ?, ?, ?, 'movie', 'Size Movie', 'Size Movie', '{}', 'tmdb', 'by_hand', 'active')`, ts, by, ts, by)
		ver := ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main) VALUES (?, ?, ?, ?, ?, 'Main', ?, ?, 'Size Movie', 1)`, ts, by, ts, by, title, rules, lf)
		for name, st := range map[string]string{"big.mkv": "replaced", "small.mkv": "present", "gone.mkv": "gone"} {
			size := int64(1000)
			if st != "present" {
				size = 33000
			}
			ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, size, added_at, origin, placed_by, state) VALUES (?, ?, ?, ?, '[]', ?, ?, ?, ?, 'imported', 'copy', ?)`,
				ts, by, ts, by, ver, name, size, ts, st)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := WithTitleFacts(context.Background(), s.Read())
	if err != nil {
		t.Fatal(err)
	}
	if got := factsOf(ctx).sizes[title]; got != 1000 {
		t.Fatalf("list size = %d; want 1000", got)
	}
}
