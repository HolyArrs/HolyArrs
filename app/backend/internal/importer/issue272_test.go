package importer

import (
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A finished Usenet folder with a clearly named episode, its sample and the
// usual side files: the episode is placed and the sample is refused.
func TestPlanSceneEpisodeFolderWithSample(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	eps := []Episode{{Ref: mediafiles.EpisodeRef{ItemID: 8, Season: 2, Episode: 8}, Name: "Handsome Touched Me", SlotID: 28, Wanted: true}}
	tgt := ShowTarget{LibraryPath: "/media/tv", FolderName: "Landman (2024)", Show: mediafiles.TitleRef{Name: "Landman", Year: 2024},
		Order: mediafiles.OrderAired, Naming: &ns, Episodes: eps, ReleaseName: "Landman.S02E08.720p.BluRay.x264-GREASE"}
	dir := "/media/downloads/complete/Landman.S02E08.720p.BluRay.x264-GREASE"
	cands := []Candidate{
		{Path: filepath.Join(dir, "Landman.S02E08.720p.BluRay.x264-GREASE.mkv"), Size: 1_670_000_000},
		{Path: filepath.Join(dir, "Landman.S02E08.720p.BluRay.x264-GREASE.nfo"), Size: 4_000},
		{Path: filepath.Join(dir, "landman.s02e08.720p.bluray.x264-grease.proof.jpg"), Size: 200_000},
		{Path: filepath.Join(dir, "landman.s02e08.720p.bluray.x264-grease.sample.mkv"), Size: 39_000_000},
		{Path: filepath.Join(dir, "landman.s02e08.720p.bluray.x264-grease.sfv"), Size: 100},
		{Path: filepath.Join(dir, "www.scenenzb.org.txt"), Size: 100},
	}
	steps := PlanEpisodes(cands, tgt)
	if steps[0].Method != "hardlink" || len(steps[0].TargetSlotIDs) != 1 || steps[0].TargetSlotIDs[0] != 28 {
		t.Fatalf("episode: %+v", steps[0])
	}
	if steps[3].Method != "refuse" {
		t.Fatalf("sample: %+v", steps[3])
	}
	for i, st := range steps {
		if i != 0 && st.Method == "hardlink" {
			t.Fatalf("placed a side file: %+v", st)
		}
	}
}
