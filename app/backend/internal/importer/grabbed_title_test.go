package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// the group leaves the show's name off its file names ("[Judas] Beerus -
// S01E05.mkv"). The download was grabbed for this show, so the import takes
// the show from the grab and only the episode from the file, as Sonarr does.
func TestPlanTakesShowFromGrabWhenFileNamesAnother(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	eps := []Episode{
		{Ref: mediafiles.EpisodeRef{ItemID: 5, Season: 1, Episode: 5}, Name: "Five", SlotID: 15, Wanted: true},
	}
	show := mediafiles.TitleRef{Name: "Dragon Ball Super: Beerus"}
	rel := "[Judas] Dragon Ball Super  - Beerus - S01E05 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)"
	tgt := ShowTarget{LibraryPath: "/lib", FolderName: "Dragon Ball Super Beerus", Show: show,
		Order: mediafiles.OrderAired, Naming: &ns, Episodes: eps, ReleaseName: rel}
	file := "/dl/" + rel + "/[Judas] Beerus - S01E05.mkv"
	steps := PlanEpisodes([]Candidate{{Path: file, Size: 1}}, tgt)
	if steps[0].Method != "hardlink" || len(steps[0].TargetSlotIDs) != 1 || steps[0].TargetSlotIDs[0] != 15 || !strings.Contains(steps[0].TargetPath, "S01E05") {
		t.Fatalf("grabbed file without the show's name: %+v", steps[0])
	}

	// A download with no grab (added by hand) still needs the file to name the show.
	tgt.ReleaseName = ""
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/[Judas] Beerus - S01E05.mkv", Size: 1}}, tgt); s[0].Method == "hardlink" {
		t.Fatalf("no grab: %+v", s[0])
	}
	// A grab found by ID under another name still vouches for the show.
	tgt.ReleaseName = "[Judas] Doragon Boru Supa - S01E05 [1080p]"
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/[Judas] Beerus - S01E05.mkv", Size: 1}}, tgt); s[0].Method != "hardlink" {
		t.Fatalf("grab under another name: %+v", s[0])
	}
	// A file that names another show is still refused, grab or not.
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/[Judas] One Piece - S01E05.mkv", Size: 1}}, tgt); s[0].Method == "hardlink" {
		t.Fatalf("another show: %+v", s[0])
	}
	// The episode still comes from the file: a number the show doesn't want is not placed.
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/[Judas] Beerus - S01E09.mkv", Size: 1}}, tgt); s[0].Method == "hardlink" {
		t.Fatalf("unwanted episode: %+v", s[0])
	}
}

// The release's group and tags are not a name for the show.
func TestShortNameIgnoresReleaseTags(t *testing.T) {
	tgt := ShowTarget{Show: mediafiles.TitleRef{Name: "Dragon Ball Super: Beerus"},
		ReleaseName: "[Judas] Dragon Ball Super  - Beerus - S01E05 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)"}
	for _, title := range []string{"Judas", "Weekly", "Multi Subs"} {
		if shortNameOf(title, tgt) {
			t.Errorf("%q passed as the show", title)
		}
	}
	if !shortNameOf("Beerus", tgt) {
		t.Error("Beerus refused")
	}
}
