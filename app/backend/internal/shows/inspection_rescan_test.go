package shows

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestShowAdoptionAndFoundFilesUseInspectedHeaders(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	adoptedRel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 1", "Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv"))
	writeInspectionMP4(t, filepath.Join(lf.Path, filepath.FromSlash(adoptedRel)))
	res, err := svc.Adopt(context.Background(), lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt show: %+v %v", res, err)
	}
	var adopted mediafiles.File
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", adoptedRel).Scan(&adopted.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), s.Read(), "files", adopted.ID, &adopted); err != nil {
		t.Fatal(err)
	}
	if adopted.MediaInfo["video_codec"] != "hevc" || fmt.Sprint(adopted.MediaInfo["audio_languages"]) != "[fr]" {
		t.Fatalf("show adoption trusted filename instead of headers: %+v", adopted.MediaInfo)
	}

	foundRel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab.Show.S02E03.XviD.English.mkv"))
	writeInspectionMP4(t, filepath.Join(lf.Path, filepath.FromSlash(foundRel)))
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	var found mediafiles.File
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", foundRel).Scan(&found.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), s.Read(), "files", found.ID, &found); err != nil {
		t.Fatal(err)
	}
	if found.Origin != "adopted" || found.MediaInfo["video_codec"] != "hevc" || fmt.Sprint(found.MediaInfo["audio_languages"]) != "[fr]" {
		t.Fatalf("found show facts: %+v", found)
	}
}

// TestRescanRecordsNewEpisodeFile: a new episode file dropped into an
// adopted show's folder is linked to its empty episode on the next rescan,
// once.
func TestRescanRecordsNewEpisodeFile(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	id := res[0].TitleID
	// Adopting leaves some files alone (extras, a sample); the first rescan
	// records what is there so the next one counts only the new file.
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, "Lab Show (2025)", "Season 2", "Lab Show S02E03.mkv"), "new episode file")
	first, err := svc.Rescan(ctx, domain.System)
	if err != nil || first.Found != 1 {
		t.Fatalf("first rescan %+v %v", first, err)
	}
	rel := filepath.ToSlash(filepath.Join("Lab Show (2025)", "Season 2", "Lab Show S02E03.mkv"))
	var fid int64
	if err := s.Read().QueryRow("SELECT id FROM files WHERE relative_path = ?", rel).Scan(&fid); err != nil {
		t.Fatalf("the new episode file was not recorded: %v", err)
	}
	f := fileRow(t, s, fid)
	if f.Origin != "adopted" || len(f.SlotIDs) != 1 || f.SlotIDs[0] != episodes(t, s, id)["S02E03"].SlotID {
		t.Fatalf("found file %+v, want it adopted on the S02E03 slot", f)
	}
	second, err := svc.Rescan(ctx, domain.System)
	if err != nil || second.Found != 0 {
		t.Fatalf("second rescan %+v %v", second, err)
	}
}
