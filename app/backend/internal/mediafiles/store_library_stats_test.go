package mediafiles

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// statsFile adds a title of kind with one main file per entry of infos
// (size 100 each) and returns the title id.
func statsFile(t *testing.T, s *database.Store, kind, name string, quality Quality, infos ...map[string]any) int64 {
	t.Helper()
	var id int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var rulesID int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rulesID); err != nil {
			rules := &PickRules{Name: "rules", Kinds: []string{"movie"}, Revision: 1, Body: PickRulesBody{}}
			if rulesID, err = database.Insert(tx, "pick_rules", rules, database.System); err != nil {
				return err
			}
		}
		folder := &LibraryFolder{Path: t.TempDir(), Kind: map[string]string{"artist": "music"}[kind], Label: name, Health: "ok", SameDiskAs: []int64{}}
		if folder.Kind == "" {
			folder.Kind = kind
		}
		folderID, err := database.Insert(tx, "library_folders", folder, database.System)
		if err != nil {
			return err
		}
		title := Title{Kind: kind, Name: name, SortName: name, Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", GateDriver: "none", WhyAdded: "by_hand", Artwork: []string{}, State: "active", Status: "released", MusicTypes: []string{}}
		if _, err := database.Insert(tx, "titles", &title, database.System); err != nil {
			return err
		}
		id = title.ID
		version := Version{TitleID: title.ID, Label: "Main", PickRulesID: rulesID, LibraryFolderID: folderID, FolderName: name, KeepUpToDate: true, IsMain: true, AudioLanguages: []string{}}
		if _, err := database.Insert(tx, "versions", &version, database.System); err != nil {
			return err
		}
		for i, info := range infos {
			f := File{VersionID: version.ID, SlotIDs: []int64{}, RelativePath: name + strconv.Itoa(i), Size: 100, AddedAt: time.Now().UTC(), Quality: quality,
				Languages: []string{}, MediaInfo: info, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", &f, database.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func bucket(t *testing.T, k LibraryKindStats, fact, value string) LibraryStatBucket {
	t.Helper()
	for _, b := range k.Facts[fact] {
		if b.Value == value {
			return b
		}
	}
	t.Fatalf("%s has no %s %q: %+v", k.Kind, fact, value, k.Facts[fact])
	return LibraryStatBucket{}
}

func kindOf(t *testing.T, st LibraryStats, kind string) LibraryKindStats {
	t.Helper()
	for _, k := range st.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	t.Fatalf("no kind %s in %+v", kind, st.Kinds)
	return LibraryKindStats{}
}

func TestLibraryStatsCountsFilesByFact(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	hevc4k := map[string]any{"inspection_status": "complete", "container": "matroska", "video_codec": "hevc", "resolution": "2160p", "hdr": "DV+HDR10", "audio_codec": "truehd", "audio_channels": "7.1"}
	h264 := map[string]any{"inspection_status": "partial", "container": "mp4", "video_codec": "h264", "resolution": "1080p", "audio_codec": "aac", "audio_channels": "2.0"}
	statsFile(t, s, "movie", "Dune", Quality{}, hevc4k)
	statsFile(t, s, "movie", "Heat", Quality{}, h264)
	statsFile(t, s, "movie", "New", Quality{}, map[string]any{})
	// A crafted header: raw text must never come back.
	statsFile(t, s, "movie", "Odd", Quality{}, map[string]any{"inspection_status": "complete", "container": "<script>x</script>", "video_codec": "evil", "resolution": "9999p", "hdr": "LOUD", "audio_codec": "zz", "audio_channels": "64.4"})
	statsFile(t, s, "movie", "Broken", Quality{}, map[string]any{"inspection_status": "error"})
	statsFile(t, s, "show", "Severance", Quality{}, h264, h264, hevc4k)
	statsFile(t, s, "artist", "Bjork", Quality{Source: "flac"}, map[string]any{}, map[string]any{})

	st, err := LibraryStatsOf(context.Background(), s, nil)
	if err != nil {
		t.Fatal(err)
	}
	movies := kindOf(t, st, "movies")
	if movies.Titles != 5 || movies.Files != 5 || movies.Bytes != 500 {
		t.Fatalf("movies %+v", movies)
	}
	if b := bucket(t, movies, "video_codec", "hevc"); b.Titles != 1 || b.Files != 1 || b.Bytes != 100 {
		t.Fatalf("hevc %+v", b)
	}
	if b := bucket(t, movies, "video_codec", "unchecked"); b.Files != 1 {
		t.Fatalf("unchecked %+v", b)
	}
	if b := bucket(t, movies, "video_codec", "unknown"); b.Files != 1 {
		t.Fatalf("unknown %+v", b)
	}
	for _, fact := range []string{"video_codec", "resolution", "hdr", "container", "audio_codec", "audio_channels"} {
		if b := bucket(t, movies, fact, "other"); b.Files != 1 {
			t.Fatalf("%s other %+v", fact, b)
		}
		for _, b := range movies.Facts[fact] {
			if !statValue.MatchString(b.Value) {
				t.Fatalf("%s value %q is not a bucket name", fact, b.Value)
			}
		}
	}
	bucket(t, movies, "hdr", "dv+hdr10")
	bucket(t, movies, "hdr", "sdr")
	bucket(t, movies, "resolution", "2160p")
	bucket(t, movies, "container", "matroska")
	bucket(t, movies, "audio_channels", "7.1")

	shows := kindOf(t, st, "shows")
	if shows.Titles != 1 || shows.Files != 3 || shows.Bytes != 300 {
		t.Fatalf("shows %+v", shows)
	}
	if b := bucket(t, shows, "video_codec", "h264"); b.Titles != 1 || b.Files != 2 || b.Bytes != 200 {
		t.Fatalf("show h264 %+v", b)
	}

	music := kindOf(t, st, "music")
	if music.Files != 2 || music.Titles != 1 {
		t.Fatalf("music %+v", music)
	}
	if _, ok := music.Facts["video_codec"]; ok {
		t.Fatalf("music has video facts: %+v", music.Facts)
	}
	if b := bucket(t, music, "container", "flac"); b.Files != 2 {
		t.Fatalf("music container %+v", b)
	}
	if b := bucket(t, music, "audio_codec", "flac"); b.Files != 2 {
		t.Fatalf("music codec %+v", b)
	}
}

func TestLibraryStatsOnlyShownKinds(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	statsFile(t, s, "movie", "Dune", Quality{}, map[string]any{})
	st, err := LibraryStatsOf(context.Background(), s, func(kind string) bool { return kind == "show" })
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Kinds) != 1 || st.Kinds[0].Kind != "shows" || st.Kinds[0].Files != 0 {
		t.Fatalf("kinds %+v", st.Kinds)
	}
	if st.Kinds[0].Facts["video_codec"] == nil {
		t.Fatalf("an empty kind still lists its facts: %+v", st.Kinds[0])
	}
}

// A 10,000-title Library answers in well under a second.
func TestLibraryStatsBigLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("big library")
	}
	s, _ := openTest(t, database.Options{})
	statsFile(t, s, "movie", "Seed", Quality{}, map[string]any{})
	info := `{"inspection_status":"complete","container":"matroska","video_codec":"hevc","resolution":"1080p","audio_codec":"eac3","audio_channels":"5.1"}`
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []string{
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 10000)
			INSERT INTO titles(created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added, state, status)
			SELECT created_at, created_by, updated_at, updated_by, CASE WHEN i % 2 THEN 'movie' ELSE 'show' END, 'T' || i, 't' || i, metadata_source, why_added, state, status
			FROM n, (SELECT * FROM titles LIMIT 1)`,
			`INSERT INTO versions(created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
			SELECT v.created_at, v.created_by, v.updated_at, v.updated_by, t.id, 'Main', v.pick_rules_id, v.library_folder_id, t.name, 1
			FROM titles t, (SELECT * FROM versions LIMIT 1) v WHERE t.name LIKE 'T%'`,
			`INSERT INTO files(created_at, created_by, updated_at, updated_by, version_id, relative_path, size, added_at, media_info, origin, placed_by)
			SELECT v.created_at, v.created_by, v.updated_at, v.updated_by, v.id, 'f' || k.i, 1000, v.created_at, ?, 'imported', 'move'
			FROM versions v, (SELECT 1 AS i UNION ALL SELECT 2) k WHERE v.label = 'Main' AND v.folder_name LIKE 'T%'`,
		} {
			var args []any
			if strings.Contains(q, "?") {
				args = []any{info}
			}
			if _, err := tx.Exec(q, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	st, err := LibraryStatsOf(context.Background(), s, nil)
	if took := time.Since(start); err != nil || took > time.Second {
		t.Fatalf("took %v: %v", took, err)
	}
	if m := kindOf(t, st, "movies"); m.Titles != 5001 || m.Files != 10001 {
		t.Fatalf("movies %+v", m)
	}
}
