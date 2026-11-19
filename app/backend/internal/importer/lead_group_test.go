package importer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Scene RAR sets hold a file named "group-title...": the group leads the
// name. Once unpacked, the file sits in the unpack folder, so neither its
// folder nor its own name gives the Title unless the lead group is dropped.
func TestPlanEpisodesDropsLeadGroup(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	year := 2024
	tgt := ShowTarget{LibraryPath: "/lib", FolderName: "The Agency (2024)", Show: mediafiles.TitleRef{Name: "The Agency", Year: year},
		Order: mediafiles.OrderAired, Naming: &ns, ReleaseName: "The.Agency.2024.S01E02.1080p.BluRay.x264-BRAVERY",
		Episodes: []Episode{{Ref: mediafiles.EpisodeRef{ItemID: 2, Season: 1, Episode: 2}, Name: "Two", SlotID: 12, Wanted: true}}}
	file := "/dl/.holyarrs-unpack-7/set0/bravery-the.agency.2024.s01e02.1080p.bluray.x264.mkv"
	s := PlanEpisodes([]Candidate{{Path: file, Size: 1}}, tgt)[0]
	if s.Method != "hardlink" || len(s.TargetSlotIDs) != 1 || s.TargetSlotIDs[0] != 12 || !strings.Contains(s.TargetPath, "S01E02") {
		t.Fatalf("lead group: %+v", s)
	}
	// A lead word in front of another show's name is still refused.
	other := PlanEpisodes([]Candidate{{Path: "/dl/.holyarrs-unpack-7/set0/bravery-the.office.s01e02.1080p.bluray.x264.mkv", Size: 1}}, tgt)[0]
	if other.Method != "refuse" || !strings.Contains(strings.Join(other.Why, " "), "bravery-the office") {
		t.Fatalf("another show: %+v", other)
	}
}

func TestPlanDropsLeadGroup(t *testing.T) {
	lib := t.TempDir()
	tg := target(lib)
	dir := filepath.Join(t.TempDir(), ".holyarrs-unpack-7", "set0")
	s := Plan([]Candidate{{Path: filepath.Join(dir, "grp-dune.part.two.2024.1080p.bluray.x264.mkv"), Size: 1000}}, tg)[0]
	if s.Method != "hardlink" {
		t.Fatalf("lead group: %+v", s)
	}
	if s := Plan([]Candidate{{Path: filepath.Join(dir, "grp-arrival.2016.1080p.bluray.x264.mkv"), Size: 1000}}, tg)[0]; s.Method != "refuse" {
		t.Fatalf("another movie: %+v", s)
	}
}
