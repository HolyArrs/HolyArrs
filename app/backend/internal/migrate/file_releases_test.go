package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The names the release tests use: a file Sonarr named with the group in
// brackets, one Sonarr renamed, and one with no scene name.
const (
	ringsFile   = "The Lord of the Rings - The Rings of Power (2022) S02E02 (1080p AMZN WEB-DL H265 SDR DDP Atmos 5.1 English - HONE).mkv"
	renamedFile = "Show - S01E01 - Pilot WEBDL-1080p.mkv"
	renamedFrom = "Show.S01E01.Pilot.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb"
	plainFile   = "Show - S01E03 - Third WEBDL-1080p.mkv"
)

// wantFile finds the TitleFile named name.
func wantFile(t *testing.T, files []TitleFile, name string) TitleFile {
	t.Helper()
	for _, f := range files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no file %q in %+v", name, files)
	return TitleFile{}
}

// Sonarr's episode file list and Radarr's movie file carry the scene name
// and group of each file: both come over with the file.
func TestReleasesFromAPI(t *testing.T) {
	u, key := sonarrAPIWith(t, false, map[string]any{
		"/api/v3/episodefile": []any{
			map[string]any{"id": 1, "relativePath": "Season 01/" + renamedFile, "quality": map[string]any{"quality": map[string]any{"name": "WEBDL-1080p"}},
				"sceneName": renamedFrom, "releaseGroup": "NTb"},
			map[string]any{"id": 2, "relativePath": "Season 02/" + ringsFile, "quality": map[string]any{"quality": map[string]any{"name": "WEBDL-1080p"}},
				"sceneName": "", "releaseGroup": "HONE"},
		},
		"/api/v3/episode": []any{},
	})
	bb := findTitle(t, read(t, api(Sonarr, u, key)), "Breaking Bad")
	if f := wantFile(t, bb.Files, renamedFile); f.SceneName != renamedFrom || f.Group != "NTb" || f.Quality != "WEBDL-1080p" {
		t.Fatalf("renamed %+v", f)
	}
	if f := wantFile(t, bb.Files, ringsFile); f.SceneName != "" || f.Group != "HONE" {
		t.Fatalf("rings %+v", f)
	}
	u, key = radarrAPIWith(t, map[string]any{"/api/v3/movie": []any{
		map[string]any{"id": 1, "tmdbId": 603, "title": "The Matrix", "year": 1999, "path": "/movies/The Matrix (1999)", "monitored": true, "qualityProfileId": 1,
			"movieFile": map[string]any{"id": 4, "relativePath": "The Matrix (1999) Bluray-1080p.mkv", "quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p"}},
				"sceneName": "The.Matrix.1999.1080p.BluRay.x264-SPARKS", "releaseGroup": "SPARKS"}},
	}})
	m := findTitle(t, read(t, api(Radarr, u, key)), "The Matrix")
	if f := wantFile(t, m.Files, "The Matrix (1999) Bluray-1080p.mkv"); f.SceneName != "The.Matrix.1999.1080p.BluRay.x264-SPARKS" || f.Group != "SPARKS" {
		t.Fatalf("movie %+v", f)
	}
}

// A backup keeps them as the SceneName and ReleaseGroup columns of
// EpisodeFiles and MovieFiles.
func TestReleasesFromBackup(t *testing.T) {
	sonarr := append(sonarrDB(false),
		`CREATE TABLE EpisodeFiles (Id INTEGER PRIMARY KEY, SeriesId INTEGER, RelativePath TEXT, Quality TEXT, SceneName TEXT, ReleaseGroup TEXT)`,
		`INSERT INTO EpisodeFiles VALUES (1, 5, 'Season 01/`+renamedFile+`', '{"quality": 3}', '`+renamedFrom+`', 'NTb'), (2, 5, 'Season 01/`+plainFile+`', '{"quality": 3}', NULL, NULL)`)
	p := filepath.Join(t.TempDir(), "sonarr.db")
	makeDB(t, p, sonarr...)
	bb := findTitle(t, read(t, file(Sonarr, p)), "Breaking Bad")
	if f := wantFile(t, bb.Files, renamedFile); f.SceneName != renamedFrom || f.Group != "NTb" || f.Quality != "WEBDL-1080p" {
		t.Fatalf("renamed %+v", f)
	}
	if f := wantFile(t, bb.Files, plainFile); f.SceneName != "" || f.Group != "" {
		t.Fatalf("plain %+v", f)
	}
	radarr := append(radarrDB(),
		`CREATE TABLE MovieFiles (Id INTEGER PRIMARY KEY, MovieId INTEGER, RelativePath TEXT, Quality TEXT, SceneName TEXT, ReleaseGroup TEXT)`,
		`INSERT INTO MovieFiles VALUES (1, 1, 'The Matrix (1999) Bluray-1080p.mkv', '{"quality": 7}', 'The.Matrix.1999.1080p.BluRay.x264-SPARKS', 'SPARKS')`)
	p = filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, p, radarr...)
	m := findTitle(t, read(t, file(Radarr, p)), "The Matrix")
	if f := wantFile(t, m.Files, "The Matrix (1999) Bluray-1080p.mkv"); f.SceneName != "The.Matrix.1999.1080p.BluRay.x264-SPARKS" || f.Group != "SPARKS" || f.Quality != "Bluray-1080p" {
		t.Fatalf("movie %+v", f)
	}
}

// Two files that share a base name get no release: which is which is not
// known.
func TestFileReleasesSkipsRepeatedBaseNames(t *testing.T) {
	got := fileReleases([]TitleFile{{Name: "Episode 01.mkv", SceneName: "A-GRP"}, {Name: "Episode 01.mkv", SceneName: "B-GRP"}, {Name: "x.mkv", Group: "HONE"}, {Name: "y.mkv", Quality: "WEBDL-1080p"}})
	if len(got) != 1 || got["x.mkv"] != (OldRelease{Group: "HONE"}) {
		t.Fatalf("%+v", got)
	}
}

// groupRules makes every Pick rules score the groups NTb (100) and HONE
// (200) and nothing else, good enough from 1080p web at 100 points.
func groupRules(t *testing.T, st *database.Store) {
	t.Helper()
	mediafiles.SetOwnTraits([]mediafiles.OwnTrait{
		{Meta: database.Meta{ID: 1}, Name: "NTb", Source: "import", Specs: []mediafiles.OwnSpec{{Name: "NTb", Kind: "group", Value: "^NTb$", Required: true}}},
		{Meta: database.Meta{ID: 2}, Name: "HONE", Source: "import", Specs: []mediafiles.OwnSpec{{Name: "HONE", Kind: "group", Value: "^HONE$", Required: true}}},
	})
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	ctx := context.Background()
	hundred := 100
	err := st.Write(ctx, func(tx *sql.Tx) error {
		all, err := database.Select[mediafiles.PickRules](ctx, tx, "pick_rules", "1 = 1")
		if err != nil || len(all) == 0 {
			t.Fatalf("pick rules %d %v", len(all), err)
		}
		for _, pr := range all {
			pr.Body.Traits = []mediafiles.TraitPoints{{Trait: mediafiles.OwnTraitKey(1), Points: 100}, {Trait: mediafiles.OwnTraitKey(2), Points: 200}}
			pr.Body.GoodEnoughPoints = &hundred
			if err := database.Update(tx, "pick_rules", &pr, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// movedFile is a moved file with its Slot's points.
type movedFile struct {
	f      mediafiles.File
	points library.SlotPoints
}

// movedFiles reads every present main file with the points of its first Slot.
func movedFiles(t *testing.T, l *moveLab) map[string]movedFile {
	t.Helper()
	ctx := context.Background()
	files, err := database.Select[mediafiles.File](ctx, l.st.Read(), "files", "state = 'present' AND role = 'main'")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]movedFile{}
	for _, f := range files {
		var titleID int64
		if len(f.SlotIDs) == 0 || l.st.Read().QueryRow(`SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?`, f.SlotIDs[0]).Scan(&titleID) != nil {
			t.Fatalf("%s fills no Slot", f.RelativePath)
		}
		sp, err := l.mv.Library.SlotPoints(ctx, titleID, f.SlotIDs[0])
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f.RelativePath)] = movedFile{f, sp}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Moving a show over brings each file's scene name and group from Sonarr:
// the renamed file is scored by the release it came from, the bracketed
// one by Sonarr's group, and the Slots are judged good enough again. A
// file with no scene name keeps the name it is adopted with.
func TestMoveBringsSonarrReleases(t *testing.T) {
	aired := time.Date(2022, 9, 1, 0, 0, 0, 0, time.UTC)
	sh := cunk(cunkSonarr)
	sh.Seasons = []metadata.Season{{Number: 1}, {Number: 2}}
	for _, e := range [][2]int{{1, 1}, {1, 3}, {2, 2}} {
		sh.Episodes = append(sh.Episodes, metadata.Episode{Season: e[0], Number: e[1], Name: "Part", AirDate: &aired})
	}
	old := []TitleFile{
		{Name: renamedFile, Quality: "WEBDL-1080p", SceneName: renamedFrom, Group: "NTb"},
		{Name: ringsFile, Quality: "WEBDL-1080p", Group: "HONE"},
		{Name: plainFile, Quality: "WEBDL-1080p"},
	}
	l, _ := cunkSetupOld(t, cunkSonarr, []metadata.Show{sh}, old, "Season 01/"+renamedFile, "Season 02/"+ringsFile, "Season 01/"+plainFile)
	groupRules(t, l.st)
	l.move(t, Choices{})
	got := movedFiles(t, l)
	cases := []struct {
		name, release, group string
		points               int
		goodEnough           bool
	}{
		{renamedFile, renamedFrom, "NTb", 100, true},
		{ringsFile, ringsFile[:len(ringsFile)-4], "HONE", 200, true},
		{plainFile, plainFile[:len(plainFile)-4], "", 0, false},
	}
	for _, c := range cases {
		m, ok := got[c.name]
		if !ok {
			t.Fatalf("%s not adopted: %v", c.name, got)
		}
		if deref(m.f.ReleaseName) != c.release || deref(m.f.Group) != c.group {
			t.Errorf("%s: release %q group %q, want %q %q", c.name, deref(m.f.ReleaseName), deref(m.f.Group), c.release, c.group)
		}
		if m.points.Points != c.points || m.points.GoodEnough != c.goodEnough {
			t.Errorf("%s: %d points, good enough %v; want %d, %v (traits %+v)", c.name, m.points.Points, m.points.GoodEnough, c.points, c.goodEnough, m.points.Traits)
		}
	}
	// The release name now says the codec the renamed file's name left out.
	if q := got[renamedFile].f.Quality; q.Codec != "h264" || q.Source != "web_dl" || q.Resolution != 1080 {
		t.Errorf("renamed quality %+v", q)
	}
}

// Radarr's movie file comes over the same way.
func TestMoveBringsRadarrRelease(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Titles[0].Files = []TitleFile{{Name: "The.Matrix.1999.1080p.mkv", Quality: "WEBDL-1080p", SceneName: "The.Matrix.1999.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", Group: "NTb"}}
	})
	groupRules(t, l.st)
	l.move(t, Choices{})
	m, ok := movedFiles(t, l)["The.Matrix.1999.1080p.mkv"]
	if !ok {
		t.Fatal("the movie was not adopted")
	}
	if deref(m.f.ReleaseName) != "The.Matrix.1999.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb" || deref(m.f.Group) != "NTb" || m.points.Points != 100 {
		t.Fatalf("release %q group %q points %d %+v", deref(m.f.ReleaseName), deref(m.f.Group), m.points.Points, m.points.Traits)
	}
}
