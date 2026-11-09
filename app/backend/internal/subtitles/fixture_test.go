package subtitles

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// fix is a small library: a movie with one File, a show with two episodes
// that have one File each (the first also has a second Version), and an artist.
type fix struct {
	st                          *database.Store
	lib                         string
	movie, show, artist         int64
	movieItem, ep1, ep2         int64
	movieVer, showVer, show4K   int64
	movieFile, ep1File, ep2File int64
}

const sysActor = `{"kind":"system"}`

func newFix(t *testing.T) *fix {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fix{st: st, lib: t.TempDir()}
	ts := database.FormatTime(now())
	ins := func(tx *sql.Tx, q string, args ...any) int64 {
		t.Helper()
		res, err := tx.Exec(q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	err = st.Write(context.Background(), func(tx *sql.Tx) error {
		var rules int64
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		lf := ins(tx, `INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health) VALUES (?, ?, ?, ?, ?, 'movie', 'Movies', 'ok')`,
			ts, sysActor, ts, sysActor, f.lib)
		title := func(kind, name string, year int, ids string) int64 {
			return ins(tx, `INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'tmdb', 'by_hand', 'active')`, ts, sysActor, ts, sysActor, kind, name, name, year, ids)
		}
		version := func(title int64, label, folder string, main bool) int64 {
			return ins(tx, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, ts, sysActor, ts, sysActor, title, label, rules, lf, folder, main)
		}
		item := func(title int64, kind, numbers string) int64 {
			return ins(tx, `INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, numbers) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ts, sysActor, ts, sysActor, title, kind, numbers)
		}
		file := func(ver, it int64, rel, release string) int64 {
			slot := ins(tx, `INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`,
				ts, sysActor, ts, sysActor, it, ver)
			id := ins(tx, `INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, release_name, origin, placed_by)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'imported', 'copy')`, ts, sysActor, ts, sysActor, "["+itoa(slot)+"]", ver, rel, ts, release)
			if _, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", id, slot); err != nil {
				t.Fatal(err)
			}
			return id
		}
		f.movie = title("movie", "The Lantern", 2010, `{"tmdb":"101","imdb":"tt0101"}`)
		f.movieVer = version(f.movie, "HD", "The Lantern (2010)", true)
		f.movieItem = item(f.movie, "movie", "{}")
		f.movieFile = file(f.movieVer, f.movieItem, "The Lantern (2010)/The Lantern (2010) 1080p BluRay.mkv", "The.Lantern.2010.1080p.BluRay.x264-GRP")
		f.show = title("show", "Quiet Harbour", 2019, `{"tvdb":"555","tmdb":"777"}`)
		f.showVer = version(f.show, "HD", "Quiet Harbour (2019)", true)
		f.show4K = version(f.show, "4K", "Quiet Harbour (2019) [4K]", false)
		f.ep1 = item(f.show, "episode", `{"season":1,"episode":2}`)
		f.ep2 = item(f.show, "episode", `{"season":1,"episode":3}`)
		f.ep1File = file(f.showVer, f.ep1, "Quiet Harbour (2019)/Season 01/Quiet Harbour - S01E02.mkv", "Quiet.Harbour.S01E02.1080p.WEB.h264-GRP")
		f.ep2File = file(f.showVer, f.ep2, "Quiet Harbour (2019)/Season 01/Quiet Harbour - S01E03.mkv", "Quiet.Harbour.S01E03.1080p.WEB.h264-GRP")
		f.artist = title("artist", "Some Band", 0, `{}`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"The Lantern (2010)/The Lantern (2010) 1080p BluRay.mkv", "Quiet Harbour (2019)/Season 01/Quiet Harbour - S01E02.mkv",
		"Quiet Harbour (2019)/Season 01/Quiet Harbour - S01E03.mkv"} {
		p := filepath.Join(f.lib, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("not a real video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
