package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// The Library table reads size on disk and the last aired episode.
func TestLibraryFacts(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := time.Now().UTC().Truncate(time.Second)
	day := func(d int) string { return database.FormatTime(now.AddDate(0, 0, d)) }
	var show int64
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := database.FormatTime(now), `{"kind":"system"}`
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		lf := ex(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, ?, ?, ?, ?, 'show', 'TV', 'ok', '[]')`, ts, by, ts, by, t.TempDir())
		var rules int64
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		show = ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
			VALUES (?, ?, ?, ?, 'show', 'Facts Show', 'Facts Show', '{}', 'tmdb', 'by_hand', 'active')`, ts, by, ts, by)
		ver := ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main) VALUES (?, ?, ?, ?, ?, 'Main', ?, ?, 'Facts Show', 1)`, ts, by, ts, by, show, rules, lf)
		for i, d := range []int{-30, -3, 10} {
			ex(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, air_date) VALUES (?, ?, ?, ?, ?, 'episode', ?)`, ts, by, ts, by, show, day(d))
			if i < 2 {
				ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, size, added_at, origin, placed_by) VALUES (?, ?, ?, ?, '[]', ?, ?, ?, ?, 'imported', 'copy')`,
					ts, by, ts, by, ver, "e"+lbItoa(int64(i))+".mkv", int64(1000*(i+1)), ts)
			}
		}
		// Copies that were replaced or deleted stay as rows but no longer count.
		for _, st := range []string{"replaced", "gone"} {
			ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, size, added_at, origin, placed_by, state) VALUES (?, ?, ?, ?, '[]', ?, ?, 90000, ?, 'imported', 'copy', ?)`,
				ts, by, ts, by, ver, "old-"+st+".mkv", ts, st)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	size, err := mediafiles.TitleSize(context.Background(), s.Read(), show)
	if err != nil || size != 3000 {
		t.Fatalf("size = %d, %v; want 3000", size, err)
	}
	last, err := shows.LastAired(context.Background(), s.Read(), show, now)
	if err != nil || last == nil || !last.Equal(now.AddDate(0, 0, -3)) {
		t.Fatalf("last aired = %v, %v; want three days ago", last, err)
	}
	if none, _ := shows.LastAired(context.Background(), s.Read(), show+99, now); none != nil {
		t.Fatalf("unknown show has a last aired date: %v", none)
	}
}

// The extras answer carries the server's wide banner for the Library rows,
// as an artwork cache link.
func TestExtrasBanner(t *testing.T) {
	t.Parallel()
	a := buildHandlers(services{Version: "test", ImageBase: "https://api.example.test/v1/img", ExtraArt: func(_ context.Context, kind, id string) (metadata.ServerArt, error) {
		return metadata.ServerArt{Banner: "https://api.example.test/v1/img/title-banner/tv-1/1000.webp"}, nil
	}})
	r := httptest.NewRequest("GET", "/api/v1/extras/show/1", nil)
	r.SetPathValue("kind", "show")
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	var got metadata.TitleExtras
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Banner != "/api/v1/artwork/img/title-banner/tv-1/1000.webp" {
		t.Fatalf("banner = %q, %v (%s)", got.Banner, err, w.Body.String())
	}
}
