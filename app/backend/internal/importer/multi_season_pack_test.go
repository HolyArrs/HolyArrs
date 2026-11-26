package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A pack holding two seasons is placed in one import: each file goes to the
// season folder of its own episode, and a file for an episode no longer
// wanted is left in the download instead of filling the Library.
func TestPlanMultiSeasonPackSplitsBySeason(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	eps := []Episode{
		{Ref: mediafiles.EpisodeRef{ItemID: 1, Season: 1, Episode: 1}, Name: "One", SlotID: 11, Wanted: true},
		{Ref: mediafiles.EpisodeRef{ItemID: 2, Season: 2, Episode: 1}, Name: "Two", SlotID: 21, Wanted: true},
		{Ref: mediafiles.EpisodeRef{ItemID: 3, Season: 2, Episode: 2}, Name: "Three", SlotID: 22},
	}
	pack := "Castle.S01-S02.1080p.WEB-DL.H.264-GRP"
	tgt := ShowTarget{LibraryPath: "/lib", FolderName: "Castle", Show: mediafiles.TitleRef{Name: "Castle"},
		Order: mediafiles.OrderAired, Naming: &ns, Episodes: eps, ReleaseName: pack}
	steps := PlanEpisodes([]Candidate{
		{Path: "/dl/" + pack + "/Castle.S01E01.1080p.WEB-DL.H.264-GRP.mkv", Size: 1},
		{Path: "/dl/" + pack + "/Castle.S02E01.1080p.WEB-DL.H.264-GRP.mkv", Size: 1},
		{Path: "/dl/" + pack + "/Castle.S02E02.1080p.WEB-DL.H.264-GRP.mkv", Size: 1},
	}, tgt)
	for i, want := range []struct {
		slot   int64
		folder string
	}{{11, "Season 1"}, {21, "Season 2"}} {
		s := steps[i]
		if s.Method != "hardlink" || len(s.TargetSlotIDs) != 1 || s.TargetSlotIDs[0] != want.slot || !strings.Contains(s.TargetPath, want.folder) {
			t.Errorf("file %d: %+v", i+1, s)
		}
	}
	// The file stays in the download folder untouched: skipped, not refused or deleted.
	if steps[2].Method != "skip" || len(steps[2].TargetSlotIDs) != 0 {
		t.Errorf("an episode no longer wanted was placed: %+v", steps[2])
	}
}
