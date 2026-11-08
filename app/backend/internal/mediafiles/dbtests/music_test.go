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

// TestMusicMigrationKeepsMoviesAndShows upgrades a database made by the
// schema before music (up to 0023, the current schema) that holds a movie
// and a show with their Items, Versions, Slots and Files: every row
// survives, a backup is taken, and the music columns, the pressings and
// lookup_cache tables, the refresh_artists schedule and the music settings
// are there.
func TestMusicMigrationKeepsMoviesAndShows(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 23)})
	if err != nil {
		t.Fatal(err)
	}
	const meta = `'2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}'`
	err = old.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO library_folders (id, created_at, created_by, updated_at, updated_by, path, kind, label, health) VALUES (1, ` + meta + `, 'C:/m', 'movie', 'Movies', 'ok')`,
			`INSERT INTO library_folders (id, created_at, created_by, updated_at, updated_by, path, kind, label, health) VALUES (2, ` + meta + `, 'C:/s', 'show', 'Shows', 'ok')`,
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, status, state, gate_driver)
				VALUES (1, ` + meta + `, 'movie', 'Old Movie', 'old movie', '{"tmdb":"1"}', 'tmdb', 'by_hand', 'released', 'active', 'digital')`,
			`INSERT INTO titles (id, created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, episode_order, season_choice, why_added, status, state, gate_driver)
				VALUES (2, ` + meta + `, 'show', 'Old Show', 'old show', '{"tmdb":"2"}', 'tmdb', 'aired', 'all', 'by_hand', 'running', 'active', 'none')`,
			`INSERT INTO parts (id, created_at, created_by, updated_at, updated_by, title_id, kind, number, name) VALUES (1, ` + meta + `, 2, 'season', 1, 'Season 1')`,
			`INSERT INTO items (id, created_at, created_by, updated_at, updated_by, title_id, kind) VALUES (1, ` + meta + `, 1, 'movie')`,
			`INSERT INTO items (id, created_at, created_by, updated_at, updated_by, title_id, part_id, kind, numbers) VALUES (2, ` + meta + `, 2, 1, 'episode', '{"season":1,"episode":1}')`,
			`INSERT INTO versions (id, created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (1, ` + meta + `, 1, 'Main', (SELECT min(id) FROM pick_rules), 1, 'Old Movie (2020)', 1)`,
			`INSERT INTO versions (id, created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (2, ` + meta + `, 2, 'Main', (SELECT min(id) FROM pick_rules), 2, 'Old Show (2020)', 1)`,
			`INSERT INTO slots (id, created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (1, ` + meta + `, 1, 1, 'in_library')`,
			`INSERT INTO slots (id, created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (2, ` + meta + `, 2, 2, 'in_library')`,
			`INSERT INTO files (id, created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by, role, state)
				VALUES (1, ` + meta + `, '[1]', 1, 'Old Movie (2020)/a.mkv', '2026-10-01T00:00:00.000000000Z', 'imported', 'hardlink', 'main', 'present')`,
			`INSERT INTO files (id, created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by, role, state)
				VALUES (2, ` + meta + `, '[2]', 2, 'Old Show (2020)/Season 01/b.mkv', '2026-10-01T00:00:00.000000000Z', 'imported', 'hardlink', 'main', 'present')`,
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
		t.Fatalf("backups before 0024: %v", b)
	}
	for id, kind := range map[int64]string{1: "movie", 2: "show"} {
		var back mediafiles.Title
		if err := database.Get(ctx, s.Read(), "titles", id, &back); err != nil || back.Kind != kind || back.AlbumChoice != nil || len(back.MusicSecondary) != 0 || back.ArtistType != nil {
			t.Fatalf("%s after the migration %+v %v", kind, back, err)
		}
	}
	var season mediafiles.Part
	if err := database.Get(ctx, s.Read(), "parts", 1, &season); err != nil || season.Name != "Season 1" || len(season.SecondaryTypes) != 0 {
		t.Fatalf("season %+v %v", season, err)
	}
	for _, id := range []int64{1, 2} {
		var bs mediafiles.Slot
		if err := database.Get(ctx, s.Read(), "slots", id, &bs); err != nil || bs.State != mediafiles.SlotInLibrary {
			t.Fatalf("slot %d %+v %v", id, bs, err)
		}
		var bi mediafiles.Item
		if err := database.Get(ctx, s.Read(), "items", id, &bi); err != nil || bi.LengthMS != nil {
			t.Fatalf("item %d %+v %v", id, bi, err)
		}
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM files").Scan(&n); err != nil || n != 2 {
		t.Fatalf("files %d %v", n, err)
	}
	var every int64
	if err := s.Read().QueryRow("SELECT every_seconds FROM schedules WHERE kind = 'refresh_artists'").Scan(&every); err != nil || every != 43200 {
		t.Fatalf("refresh_artists schedule %d %v", every, err)
	}
	want := map[string]string{"what_to_pick/music_album_choice": "all", "what_to_pick/music_new_albums": "all",
		"what_to_pick/music_album_types": `["album","ep"]`, "what_to_pick/music_secondary_types": `["studio"]`,
		"what_to_pick/music_release_statuses": `["official"]`, "what_to_pick/music_pressing_rule": "common",
		"links/musicbrainz_url": "", "links/musicbrainz_cache_hours": "24"}
	for k, w := range want {
		grp, key := k[:len(k)-len(filepath.Base(k))-1], filepath.Base(k)
		var got string
		if err := s.Read().QueryRow("SELECT value FROM settings WHERE grp = ? AND key = ?", grp, key).Scan(&got); err != nil || got != w {
			t.Errorf("setting %s = %q (%v), want %q", k, got, err, w)
		}
	}
}

// TestMusicRows writes an artist with an album, two Pressings and the
// chosen Pressing's tracks with their Slots, reads them back, and checks the
// rules the database keeps: one album row per MusicBrainz id, one Pressing
// row per id per album, one chosen Pressing per album, the album choice
// list, and Pressings going with their album.
func TestMusicRows(t *testing.T) {
	s, dir := openTest(t, database.Options{})
	ctx := context.Background()
	rel := time.Date(2020, 3, 6, 0, 0, 0, 0, time.UTC)
	var artist mediafiles.Title
	var album mediafiles.Part
	var chosen, other domain.AlbumPressing
	var tracks []mediafiles.Item
	err := s.Write(ctx, func(tx *sql.Tx) error {
		lf := mediafiles.LibraryFolder{Path: dir, Kind: "music", Label: "Music", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", &lf, domain.System); err != nil {
			return err
		}
		choice, kind := "latest", "group"
		artist = mediafiles.Title{Kind: "artist", Name: "Lab Band", SortName: "lab band", Aliases: []string{}, SourceIDs: map[string]string{"musicbrainz": "a-1"},
			MetadataSource: "musicbrainz", KeepUpToDate: true, NewPartsPolicy: "future_only", ReleaseGate: "released", MusicTypes: []string{"album", "ep"},
			MusicSecondary: []string{"studio", "live"}, MusicStatuses: []string{"official"}, WhyAdded: "by_hand", Artwork: []string{},
			Status: "running", State: "active", GateDriver: "none", AlbumChoice: &choice, ArtistType: &kind}
		if _, err := database.Insert(tx, "titles", &artist, domain.System); err != nil {
			return err
		}
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		v := mediafiles.Version{TitleID: artist.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Lab Band", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		typ := "album"
		album = mediafiles.Part{TitleID: artist.ID, Kind: "album", Name: "First Light", ReleaseType: &typ, ReleaseDate: &rel, KeepUpToDate: true,
			SourceIDs: map[string]string{"musicbrainz": "rg-1"}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{"live"}, Included: true}
		if _, err := database.Insert(tx, "parts", &album, domain.System); err != nil {
			return err
		}
		chosen = domain.AlbumPressing{PartID: album.ID, MBID: "r-1", Name: "First Light", Status: "official", Country: "XW", Date: "2020-03-06",
			Formats: []string{"Digital Media"}, TrackCount: 2, Chosen: true, ChosenBy: "auto", Tracks: []domain.PressingTrack{
				{ID: "t-1", RecordingID: "rec-1", Disc: 1, Number: 1, Position: 1, Name: "Dawn", LengthMS: 200000},
				{ID: "t-2", RecordingID: "rec-2", Disc: 1, Number: 2, Position: 2, Name: "Noon", LengthMS: 180000}}}
		other = domain.AlbumPressing{PartID: album.ID, MBID: "r-2", Name: "First Light (Deluxe)", Status: "official", Country: "US", Date: "2020",
			Formats: []string{"CD", "CD"}, TrackCount: 3, ChosenBy: "auto", Tracks: []domain.PressingTrack{}}
		for _, p := range []*domain.AlbumPressing{&chosen, &other} {
			if _, err := database.Insert(tx, "pressings", p, domain.System); err != nil {
				return err
			}
		}
		for _, pt := range chosen.Tracks {
			name, ln, rec := pt.Name, pt.LengthMS, pt.RecordingID
			it := mediafiles.Item{TitleID: artist.ID, PartID: &album.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: intp(pt.Disc), Track: intp(pt.Number)},
				Name: &name, AirDate: &rel, KeepUpToDate: true, RecordingID: &rec, LengthMS: &ln, SourceIDs: map[string]string{"musicbrainz": pt.ID}}
			if _, err := database.Insert(tx, "items", &it, domain.System); err != nil {
				return err
			}
			tracks = append(tracks, it)
			if _, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var back mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", artist.ID, &back); err != nil || *back.AlbumChoice != "latest" || len(back.MusicSecondary) != 2 || back.MusicStatuses[0] != "official" || *back.ArtistType != "group" {
		t.Fatalf("artist back %+v %v", back, err)
	}
	var ba mediafiles.Part
	if err := database.Get(ctx, s.Read(), "parts", album.ID, &ba); err != nil || !ba.Included || ba.SecondaryTypes[0] != "live" || !ba.ReleaseDate.Equal(rel) {
		t.Fatalf("album back %+v %v", ba, err)
	}
	var bp domain.AlbumPressing
	if err := database.Get(ctx, s.Read(), "pressings", chosen.ID, &bp); err != nil || !bp.Chosen || len(bp.Tracks) != 2 || bp.Tracks[1].RecordingID != "rec-2" || bp.Formats[0] != "Digital Media" {
		t.Fatalf("pressing back %+v %v", bp, err)
	}
	var bt mediafiles.Item
	if err := database.Get(ctx, s.Read(), "items", tracks[1].ID, &bt); err != nil || *bt.Numbers.Track != 2 || *bt.LengthMS != 180000 || *bt.RecordingID != "rec-2" {
		t.Fatalf("track back %+v %v", bt, err)
	}

	refused := map[string]string{
		"second album rg-1":   `INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, name, source_ids) VALUES ('x', '{}', 'x', '{}', ?, 'album', 'again', '{"musicbrainz":"rg-1"}')`,
		"second pressing r-1": `INSERT INTO pressings (created_at, created_by, updated_at, updated_by, part_id, mbid, name) VALUES ('x', '{}', 'x', '{}', (SELECT id FROM parts WHERE title_id = ?), 'r-1', 'again')`,
		"two chosen":          `UPDATE pressings SET chosen = 1 WHERE part_id = (SELECT id FROM parts WHERE title_id = ?)`,
		"bad album choice":    `UPDATE titles SET album_choice = 'existing' WHERE id = ?`,
		"bad chosen_by":       `UPDATE pressings SET chosen_by = 'robot' WHERE part_id = (SELECT id FROM parts WHERE title_id = ?)`,
	}
	for name, q := range refused {
		err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, artist.ID); return err })
		if err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// Removing the album takes its Pressings, tracks and Slots with it.
	err = s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM parts WHERE id = ?", album.ID); return err })
	if err != nil {
		t.Fatal(err)
	}
	for table, q := range map[string]string{"pressings": "SELECT count(*) FROM pressings", "items": "SELECT count(*) FROM items WHERE kind = 'track'",
		"slots": "SELECT count(*) FROM slots"} {
		var n int
		if err := s.Read().QueryRow(q).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s left after the album went: %d %v", table, n, err)
		}
	}
}
