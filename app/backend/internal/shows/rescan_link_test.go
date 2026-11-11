package shows

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Files in a show's own folder are matched by season and episode whatever
// show name they carry, as Sonarr does for a series folder: another
// release's spelling or a shortened name no longer leaves a season alone.
func TestMapEpisodeFilesTrustsTheShowsFolder(t *testing.T) {
	sh := metadata.Show{Name: "Lab Show", Year: 2025, Episodes: []metadata.Episode{{Season: 1, Number: 1}, {Season: 1, Number: 2}}}
	vids := []library.AdoptVideo{{Path: "Season 1/Lab Series UK - S01E01.mkv", Size: 2 << 20}, {Path: "Season 1/LS.S01E02.720p.mkv", Size: 1 << 20}}
	for _, f := range mapEpisodeFiles(vids, sh, "aired") {
		if len(f.Episodes) != 1 {
			t.Fatalf("%s: not matched (%s)", f.Path, f.Why)
		}
	}
}

// A rescan links a file it finds in a show's folder to its episodes while
// they are still empty, the way adopting does; a file for an episode that
// already has one stays a found note.
func TestRescanLinksFoundFileOfEmptyEpisode(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	id := res[0].TitleID
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab Series UK - S02E03.mkv"))
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "the third episode")
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	ep := episodes(t, s, id)["S02E03"]
	var fid int64
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", rel).Scan(&fid); err != nil {
		t.Fatalf("the file was not recorded: %v", err)
	}
	f := fileRow(t, s, fid)
	if f.Origin != "adopted" || len(f.SlotIDs) != 1 || f.SlotIDs[0] != ep.SlotID {
		t.Fatalf("file %+v, want adopted on the S02E03 slot", f)
	}
	var fileID *int64
	if err := s.Read().QueryRow("SELECT file_id FROM slots WHERE id = ?", ep.SlotID).Scan(&fileID); err != nil || fileID == nil || *fileID != fid {
		t.Fatalf("slot file %v %v", fileID, err)
	}
	if ep.State != mediafiles.SlotInLibrary && ep.State != mediafiles.SlotBetterWanted {
		t.Fatalf("slot state %s", ep.State)
	}
	small := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 1", "S01E02.small.mkv"))
	var origin string
	if err := s.Read().QueryRow("SELECT origin FROM files WHERE relative_path = ?", small).Scan(&origin); err != nil || origin != "found" {
		t.Fatalf("second copy of a filled episode: %q %v", origin, err)
	}
	if r, err := svc.Rescan(ctx, domain.System); err != nil || r.Found != 0 {
		t.Fatalf("second rescan %+v %v", r, err)
	}
}

// A found note an earlier rescan left on a still empty episode, with or
// without its episodes, is linked by the next rescan.
func TestRescanLinksEarlierFoundNote(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab Series UK - S02E03.mkv"))
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "the third episode")
	var vid int64
	_ = s.Read().QueryRow("SELECT id FROM versions WHERE title_id = ?", res[0].TitleID).Scan(&vid)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{}, VersionID: vid, RelativePath: rel, Size: 17, Languages: []string{}, MediaInfo: map[string]any{},
			Origin: "found", PlacedBy: "none", Role: "main", State: "present"}
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	var origin string
	var n int
	_ = s.Read().QueryRow("SELECT origin, count(*) FROM files WHERE relative_path = ?", rel).Scan(&origin, &n)
	ep := episodes(t, s, res[0].TitleID)["S02E03"]
	if n != 1 || origin != "adopted" || (ep.State != mediafiles.SlotInLibrary && ep.State != mediafiles.SlotBetterWanted) {
		t.Fatalf("earlier found note: %d rows, origin %q, episode %s", n, origin, ep.State)
	}
}

// A found note the person protected stays protected once a rescan links it.
func TestRescanLinkKeepsFoundNoteProtection(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab Series UK - S02E03.mkv"))
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "the third episode")
	var vid int64
	_ = s.Read().QueryRow("SELECT id FROM versions WHERE title_id = ?", res[0].TitleID).Scan(&vid)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{}, VersionID: vid, RelativePath: rel, Size: 17, Languages: []string{}, MediaInfo: map[string]any{},
			Origin: "found", PlacedBy: "none", Role: "main", State: "present", Protected: true}
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	var origin string
	var protected bool
	_ = s.Read().QueryRow("SELECT origin, protected FROM files WHERE relative_path = ?", rel).Scan(&origin, &protected)
	if origin != "adopted" || !protected {
		t.Fatalf("linked note: origin %q, protected %v", origin, protected)
	}
}
