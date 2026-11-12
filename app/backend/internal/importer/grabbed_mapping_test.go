package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The site numbers a long season's second half as season 2, so search took
// "S02E06" for S01E18 (one long season here). The import keeps the grab's
// choice: the file with the release's numbers fills S01E18, not "nothing
// could be matched".
func TestPlanUsesGrabbedEpisodes(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	eps := []Episode{
		{Ref: mediafiles.EpisodeRef{ItemID: 6, Season: 1, Episode: 6}, Name: "Six", SlotID: 16},
		{Ref: mediafiles.EpisodeRef{ItemID: 18, Season: 1, Episode: 18}, Name: "Before Spring", SlotID: 28, Wanted: true},
	}
	rel := "You.and.I.Are.Polar.Opposites.S02E06.Before.Spring.1080p.WEB.H264-GRP"
	tgt := ShowTarget{LibraryPath: "/lib", FolderName: "You and I Are Polar Opposites", Show: mediafiles.TitleRef{Name: "You and I Are Polar Opposites"},
		Order: mediafiles.OrderAired, Naming: &ns, Episodes: eps, ReleaseName: rel}
	steps := PlanEpisodes([]Candidate{{Path: "/dl/" + rel + "/" + rel + ".mkv", Size: 1}}, tgt)
	if steps[0].Method != "hardlink" || len(steps[0].TargetSlotIDs) != 1 || steps[0].TargetSlotIDs[0] != 28 || !strings.Contains(steps[0].TargetPath, "S01E18") {
		t.Fatalf("grabbed S02E06 for S01E18: %+v", steps[0])
	}
	if !strings.Contains(strings.Join(steps[0].Why, "; "), "grabbed as S02E06") {
		t.Fatalf("why: %v", steps[0].Why)
	}

	// A file with other numbers than the grabbed release is not forced in.
	other := PlanEpisodes([]Candidate{{Path: "/dl/x/You.and.I.Are.Polar.Opposites.S02E07.1080p.WEB.H264-GRP.mkv", Size: 1}}, tgt)
	if other[0].Method == "hardlink" {
		t.Fatalf("other numbers: %+v", other[0])
	}
	// With no grabbed release name the numbers alone decide, as before.
	tgt.ReleaseName = ""
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/" + rel + ".mkv", Size: 1}}, tgt); s[0].Method == "hardlink" {
		t.Fatalf("no release: %+v", s[0])
	}
}
