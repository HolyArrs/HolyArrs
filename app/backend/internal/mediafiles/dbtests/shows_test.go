package dbtests

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func intp(n int) *int { return &n }

// TestShowsMigrationKeepsMovies upgrades a database made by the schema
// before shows (up to 0019) that holds a movie with its Item, Version, Slot
// and File: every row survives unchanged, a backup is taken, and the new
// show columns, the refresh_shows schedule and the show settings are there.
func TestShowsMigrationKeepsMovies(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 19)})
	if err != nil {
		t.Fatal(err)
	}
	// Rows as the schema before shows writes them (plain SQL: the Go
	// records already carry the new columns).
	const meta = `'2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}'`
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO library_folders (id, created_at, created_by, updated_at, updated_by, path, kind, label, health) VALUES (1, ` + meta + `, 'C:/m', 'movie', 'Movies', 'ok')`,
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, status, state, gate_driver)
				VALUES (1, ` + meta + `, 'movie', 'Old Movie', 'old movie', '{"tmdb":"1"}', 'tmdb', 'by_hand', 'released', 'active', 'digital')`,
			`INSERT INTO items (id, created_at, created_by, updated_at, updated_by, title_id, kind) VALUES (1, ` + meta + `, 1, 'movie')`,
			`INSERT INTO versions (id, created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (1, ` + meta + `, 1, 'Main', (SELECT min(id) FROM pick_rules), 1, 'Old Movie (2020)', 1)`,
			`INSERT INTO slots (id, created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (1, ` + meta + `, 1, 1, 'in_library')`,
			`INSERT INTO files (id, created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by, role, state)
				VALUES (1, ` + meta + `, '[1]', 1, 'Old Movie (2020)/a.mkv', '2026-10-01T00:00:00.000000000Z', 'imported', 'hardlink', 'main', 'present')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = old.Close()
		t.Fatal(err)
	}
	_ = old.Close()

	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if b, _ := filepath.Glob(filepath.Join(dir, "backups", "main-pre-update-*-to-*.db")); len(b) != 1 {
		t.Fatalf("backups before 0020: %v", b)
	}
	var back mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", 1, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != "Old Movie" || back.Kind != "movie" || back.SeasonChoice != nil || back.Network != nil || back.GateDriver != "digital" {
		t.Fatalf("movie after the migration %+v", back)
	}
	var bi mediafiles.Item
	if err := database.Get(ctx, s.Read(), "items", 1, &bi); err != nil || bi.Kind != "movie" || bi.AirDay != nil || bi.DroppedAt != nil || len(bi.SourceIDs) != 0 {
		t.Fatalf("movie item %+v %v", bi, err)
	}
	var bs mediafiles.Slot
	if err := database.Get(ctx, s.Read(), "slots", 1, &bs); err != nil || bs.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %+v %v", bs, err)
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM files WHERE version_id = 1").Scan(&n); err != nil || n != 1 {
		t.Fatalf("files %d %v", n, err)
	}
	var every int64
	if err := s.Read().QueryRow("SELECT every_seconds FROM schedules WHERE kind = 'refresh_shows'").Scan(&every); err != nil || every != 43200 {
		t.Fatalf("refresh_shows schedule %d %v", every, err)
	}
	want := map[string]string{"keep_specials": "false", "episode_wait_minutes": "0", "show_season_choice": "all",
		"show_new_seasons": "all", "show_episode_order": "aired", "show_details_source": "tmdb"}
	for k, w := range want {
		var got string
		if err := s.Read().QueryRow("SELECT value FROM settings WHERE grp = 'what_to_pick' AND key = ?", k).Scan(&got); err != nil || got != w {
			t.Errorf("setting %s = %q (%v), want %q", k, got, err, w)
		}
	}
}

// TestEpisodeRows writes a show with two seasons (specials included), its
// episodes and a multi-episode File, and checks the rules the database
// keeps: one season row per number, one episode row per (season, episode),
// and the season choice and episode order lists.
func TestEpisodeRows(t *testing.T) {
	s, dir := openTest(t, database.Options{})
	ctx := context.Background()
	air := time.Date(2026, 1, 5, 2, 0, 0, 0, time.UTC)
	day := "2026-01-04"
	var show mediafiles.Title
	var eps []mediafiles.Item
	var slots []int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		lf := mediafiles.LibraryFolder{Path: dir, Kind: "show", Label: "Shows", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", &lf, domain.System); err != nil {
			return err
		}
		choice, order, net, at, tz := "future", "aired", "Lab TV", "21:00", "America/New_York"
		show = mediafiles.Title{Kind: "show", Name: "Lab Show", SortName: "lab show", Aliases: []string{}, SourceIDs: map[string]string{"tmdb": "100", "tvdb": "200"},
			MetadataSource: "tmdb", EpisodeOrder: &order, KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{},
			WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", GateDriver: "none",
			SeasonChoice: &choice, Network: &net, AirTime: &at, TimeZone: &tz}
		if _, err := database.Insert(tx, "titles", &show, domain.System); err != nil {
			return err
		}
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		v := mediafiles.Version{TitleID: show.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Lab Show", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		for _, sn := range []int{0, 1} {
			p := mediafiles.Part{TitleID: show.ID, Kind: "season", Number: intp(sn), Name: "Season", KeepUpToDate: sn > 0, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
			if _, err := database.Insert(tx, "parts", &p, domain.System); err != nil {
				return err
			}
			for ep := 1; ep <= 2; ep++ {
				it := mediafiles.Item{TitleID: show.ID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: intp(sn), Episode: intp(ep)},
					AirDate: &air, AirDay: &day, KeepUpToDate: sn > 0, SourceIDs: map[string]string{"tmdb": "e1"}}
				if sn > 0 {
					it.Numbers.Absolute = intp(ep)
				}
				if _, err := database.Insert(tx, "items", &it, domain.System); err != nil {
					return err
				}
				eps = append(eps, it)
				sl := mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotNotOutYet}
				if _, err := database.Insert(tx, "slots", &sl, domain.System); err != nil {
					return err
				}
				slots = append(slots, sl.ID)
			}
		}
		// One file holds S01E01 and S01E02.
		f := mediafiles.File{SlotIDs: []int64{slots[2], slots[3]}, VersionID: v.ID, RelativePath: "Lab Show/Season 01/Lab Show - S01E01-E02.mkv",
			Languages: []string{}, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id IN (?, ?)", f.ID, slots[2], slots[3])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var back mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", show.ID, &back); err != nil || *back.SeasonChoice != "future" || *back.TimeZone != "America/New_York" {
		t.Fatalf("show back %+v %v", back, err)
	}
	var bi mediafiles.Item
	if err := database.Get(ctx, s.Read(), "items", eps[2].ID, &bi); err != nil || *bi.Numbers.Season != 1 || *bi.Numbers.Absolute != 1 || *bi.AirDay != day ||
		!bi.AirDate.Equal(air) || bi.SourceIDs["tmdb"] != "e1" {
		t.Fatalf("episode back %+v %v", bi, err)
	}
	// Both episode Slots point at the one File, and the File lists both.
	var n int
	if err := s.Read().QueryRow("SELECT count(DISTINCT file_id) FROM slots WHERE id IN (?, ?)", slots[2], slots[3]).Scan(&n); err != nil || n != 1 {
		t.Fatalf("multi-episode file: %d %v", n, err)
	}
	if err := s.Read().QueryRow("SELECT json_array_length(slot_ids) FROM files").Scan(&n); err != nil || n != 2 {
		t.Fatalf("file slot_ids %d %v", n, err)
	}

	refused := map[string]string{
		"second season 1":   `INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, number, name) VALUES ('x', '{}', 'x', '{}', ?, 'season', 1, 'again')`,
		"second S01E01":     `INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, numbers) VALUES ('x', '{}', 'x', '{}', ?, 'episode', '{"season":1,"episode":1}')`,
		"bad season choice": `UPDATE titles SET season_choice = 'pilot' WHERE id = ?`,
	}
	for name, q := range refused {
		err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, show.ID); return err })
		if err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// Albums and tracks are not bound by the season and episode rules.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for i := 0; i < 2; i++ {
			if _, err := tx.Exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, numbers) VALUES ('x', '{}', 'x', '{}', ?, 'track', '{}')`, show.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("two tracks without numbers: %v", err)
	}
}
