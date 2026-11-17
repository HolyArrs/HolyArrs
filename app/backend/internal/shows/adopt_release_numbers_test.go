package shows

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A show whose details source keeps two parts in one season: files named
// by the release-name numbers (S02E05 for stored S01E17) are adopted.
func TestMapEpisodeFilesTakesReleaseNameNumbers(t *testing.T) {
	sh := metadata.Show{Name: "Mashle", Year: 2023}
	for n := 1; n <= 25; n++ {
		e := metadata.Episode{Season: 1, Number: n, Absolute: n}
		if n > 12 {
			e.AltSeason, e.AltEpisode = 2, n-12
		}
		sh.Episodes = append(sh.Episodes, e)
	}
	got := mapEpisodeFiles([]library.AdoptVideo{{Path: "Mashle - S02E05 - Something.mkv", Size: 1 << 20}}, sh, "aired")
	if len(got[0].Keys) != 1 || got[0].Keys[0] != [2]int{1, 17} {
		t.Fatalf("got %+v, want stored S01E17", got[0])
	}
}

// A found file named by the release-name numbers lands on the stored episode.
func TestRescanFoundFileTakesReleaseNameNumbers(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	id := res[0].TitleID
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	ep := episodes(t, s, id)["S02E03"]
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET numbers = json_set(numbers, '$.alt_season', 5, '$.alt_episode', 7) WHERE id = ?", ep.ItemID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, "Lab Show (2025)", "Season 2", "Lab Show S05E07.mkv"), "new episode file")
	if r, err := svc.Rescan(ctx, domain.System); err != nil || r.Found != 1 {
		t.Fatalf("rescan %+v %v", r, err)
	}
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab Show S05E07.mkv"))
	var fid int64
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", rel).Scan(&fid); err != nil {
		t.Fatalf("not recorded: %v", err)
	}
	if f := fileRow(t, s, fid); len(f.SlotIDs) != 1 || f.SlotIDs[0] != ep.SlotID {
		t.Fatalf("found file %+v, want the S02E03 slot", f)
	}
}
