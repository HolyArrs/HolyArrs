package shows

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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
	if found.Origin != "found" || found.MediaInfo["video_codec"] != "hevc" || fmt.Sprint(found.MediaInfo["audio_languages"]) != "[fr]" {
		t.Fatalf("found show facts: %+v", found)
	}
}
