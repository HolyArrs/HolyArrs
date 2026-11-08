package library

import (
	"context"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// NAS system folders hold thumbnails, deleted files and snapshots. The
// rescan and adopt folder walks never go into them: reading them wakes
// disks and finds files that are not the person's media.
var nasSystemFolders = []string{"@eaDir", "#recycle", "@Recycle", "#snapshot", "lost+found", "$RECYCLE.BIN", "System Volume Information"}

func TestFolderWalksSkipNASSystemFolders(t *testing.T) {
	lib := t.TempDir()
	writeFile(t, filepath.Join(lib, "Heat (1995)", "heat.mkv"), "h")
	writeFile(t, filepath.Join(lib, "Some Band", "Album", "01.flac"), "a")
	for _, name := range nasSystemFolders {
		writeFile(t, filepath.Join(lib, "Heat (1995)", name, "inside.mkv"), "x")
		writeFile(t, filepath.Join(lib, "Some Band", name, "inside.flac"), "x")
	}
	vids := listVideos(lib, "Heat (1995)")
	if len(vids) != 1 || vids[0].Path != "Heat (1995)/heat.mkv" {
		t.Errorf("movie folder walk went into NAS system folders: %+v", vids)
	}
	audio := listAudio(lib, "Some Band", 100)
	if len(audio) != 1 || audio[0].Path != "Some Band/Album/01.flac" {
		t.Errorf("music folder walk went into NAS system folders: %+v", audio)
	}
}

func TestAdoptPlanSkipsNASSystemFolders(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lib := t.TempDir()
	writeFile(t, filepath.Join(lib, "Heat (1995) {tmdb-949}", "heat.avi"), "h")
	for _, name := range nasSystemFolders {
		writeFile(t, filepath.Join(lib, name, "Heat (1995)", "heat.mkv"), "x")
	}
	lf, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := byFolder(plan)
	for _, name := range nasSystemFolders {
		if _, ok := got[name]; ok {
			t.Errorf("adopt plan lists the NAS system folder %q", name)
		}
	}
	if _, ok := got["Heat (1995) {tmdb-949}"]; !ok {
		t.Errorf("adopt plan lost the real movie folder: %+v", plan.Folders)
	}
}
