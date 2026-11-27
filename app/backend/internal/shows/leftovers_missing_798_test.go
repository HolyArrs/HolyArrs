package shows

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// an episode file replaced outside the app. A rescan leaves the new
// file in the slot, not the missing row; a file it cannot place can be
// matched by hand without replace, since the missing copy fills nothing.
func TestMissingEpisodeFileDoesNotBlockNewCopy(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id := adoptCopper(t, svc, tmdb, lf)
	season := filepath.Join(lf.Path, "Copper Valley (2020)", "Season 1")
	slotFile := func(k string) *int64 {
		var fid *int64
		_ = s.Read().QueryRow("SELECT file_id FROM slots WHERE id = ?", episodes(t, s, id)[k].SlotID).Scan(&fid)
		return fid
	}
	fileOf := func(fid int64) (rel, state string) {
		_ = s.Read().QueryRow("SELECT relative_path, state FROM files WHERE id = ?", fid).Scan(&rel, &state)
		return
	}
	rescan := func() {
		t.Helper()
		for i := 0; i < 2; i++ { // the missing mark may need its own pass
			if _, err := svc.RescanTitle(ctx, id, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}

	// A named replacement: the rescan links it and the slot follows.
	if err := os.Remove(filepath.Join(season, "Copper.Valley.S01E01.1080p.WEB.mkv")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(season, "Copper.Valley.S01E01.2160p.WEB.mkv"), "new copy one")
	rescan()
	if fid := slotFile("S01E01"); fid == nil {
		t.Fatal("S01E01 has no file")
	} else if rel, state := fileOf(*fid); state != "present" || filepath.Base(rel) != "Copper.Valley.S01E01.2160p.WEB.mkv" {
		t.Fatalf("S01E01 slot on %s (%s)", rel, state)
	}

	// An unnamed replacement: a leftover that Match links without replace.
	if err := os.Remove(filepath.Join(season, "Copper.Valley.S01E02.1080p.WEB.mkv")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(season, "copper-valley-pilot-take-two.mkv"), "new copy two")
	rescan()
	l, ok := leftovers(t, svc, id)["copper-valley-pilot-take-two.mkv"]
	if !ok {
		if fid := slotFile("S01E02"); fid != nil {
			if rel, state := fileOf(*fid); state == "present" && filepath.Base(rel) == "copper-valley-pilot-take-two.mkv" {
				return // the rescan placed it already
			}
		}
		t.Fatalf("new file is not a leftover: %+v", leftovers(t, svc, id))
	}
	out, err := svc.MatchLeftovers(ctx, id, []library.LeftoverMatch{{FileID: l.FileID, EpisodeIDs: []int64{episodes(t, s, id)["S01E02"].ItemID}}}, false, domain.System)
	if err != nil || !out[0].Linked {
		t.Fatalf("match %+v %v", out, err)
	}
	if fid := slotFile("S01E02"); fid == nil || *fid != l.FileID {
		t.Fatalf("S01E02 slot %v", fid)
	}
}
