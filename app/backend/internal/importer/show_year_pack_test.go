package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A season pack forced by hand although its year is not the show's (the
// show is stored as 2010, the files say 2009) still lands on its episodes:
// the import matches a show by name and takes any year.
func TestPlanPackWithAnotherYear(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	eps := []Episode{
		{Ref: mediafiles.EpisodeRef{ItemID: 1, Season: 1, Episode: 1}, Name: "One", SlotID: 11, Wanted: true},
		{Ref: mediafiles.EpisodeRef{ItemID: 2, Season: 1, Episode: 2}, Name: "Two", SlotID: 12, Wanted: true},
	}
	pack := "Castle.2009.S01.1080p.BluRay.x264-GRP"
	tgt := ShowTarget{LibraryPath: "/lib", FolderName: "Castle", Show: mediafiles.TitleRef{Name: "Castle", Year: 2010},
		Order: mediafiles.OrderAired, Naming: &ns, Episodes: eps, ReleaseName: pack}
	steps := PlanEpisodes([]Candidate{
		{Path: "/dl/" + pack + "/Castle.2009.S01E01.1080p.BluRay.x264-GRP.mkv", Size: 1},
		{Path: "/dl/" + pack + "/Castle.2009.S01E02.1080p.BluRay.x264-GRP.mkv", Size: 1},
		{Path: "/dl/" + pack + "/Fortress.2009.S01E02.1080p.BluRay.x264-GRP.mkv", Size: 1},
	}, tgt)
	for i, slot := range []int64{11, 12} {
		if s := steps[i]; s.Method != "hardlink" || len(s.TargetSlotIDs) != 1 || s.TargetSlotIDs[0] != slot || !strings.Contains(s.TargetPath, []string{"S01E01", "S01E02"}[i]) {
			t.Errorf("file %d: %+v", i+1, s)
		}
	}
	if steps[2].Method == "hardlink" {
		t.Errorf("a file named for another show was placed: %+v", steps[2])
	}
}
