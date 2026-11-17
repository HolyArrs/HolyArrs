package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A show on a chosen episode order names its files by that order's numbers.
func TestEpisodePathByOrder(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	eps := []Episode{{Ref: mediafiles.EpisodeRef{Season: 3, Episode: 4, AltSeason: 3, AltEpisode: 3}, Name: "Episode 4"}}
	name := "Trapped.S03E03.1080p.WEB.H264-GRP"
	p := releasename.Parse(name, releasename.Hints{Show: true})
	tgt := ShowTarget{FolderName: "Trapped (2015)", Show: mediafiles.TitleRef{Name: "Trapped", Year: 2015}, Order: mediafiles.OrderAired, Naming: &ns}
	if got := tgt.episodePath(p, name+".mkv", eps); !strings.Contains(got, "S03E04") {
		t.Fatalf("TMDB numbers: %s", got)
	}
	tgt.ByOrder = true
	if got := tgt.episodePath(p, name+".mkv", eps); !strings.Contains(got, "S03E03") {
		t.Fatalf("order numbers: %s", got)
	}
	if eps[0].Ref.Episode != 4 {
		t.Fatalf("the episode list is left as it was: %+v", eps)
	}
}

// A file for an episode the order leaves out is refused when its TMDB
// numbers would give it the name of another episode in the order.
func TestPlanRefusesOrderNameClash(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	eps := []Episode{
		{Ref: mediafiles.EpisodeRef{ItemID: 7, Season: 3, Episode: 7, AirDay: "2019-12-07"}, Name: "Episode 7", SlotID: 17, Wanted: true},
		{Ref: mediafiles.EpisodeRef{ItemID: 8, Season: 3, Episode: 8, AltSeason: 3, AltEpisode: 7, AirDay: "2019-12-08"}, Name: "Episode 8", SlotID: 18, Wanted: true},
		{Ref: mediafiles.EpisodeRef{ItemID: 9, Season: 3, Episode: 9, AirDay: "2019-12-09"}, Name: "Episode 9", SlotID: 19, Wanted: true},
	}
	tgt := ShowTarget{LibraryPath: "/lib", FolderName: "Trapped (2015)", Show: mediafiles.TitleRef{Name: "Trapped", Year: 2015}, Order: mediafiles.OrderAired,
		Naming: &ns, Episodes: eps, ByOrder: true}
	steps := PlanEpisodes([]Candidate{{Path: "/dl/Trapped.2019.12.07.1080p.WEB.H264-GRP.mkv", Size: 1}, {Path: "/dl/Trapped.2019.12.09.1080p.WEB.H264-GRP.mkv", Size: 1}}, tgt)
	if steps[0].Method != "refuse" || !strings.Contains(strings.Join(steps[0].Why, "; "), "not in the chosen episode order") {
		t.Fatalf("clash: %+v", steps[0])
	}
	if steps[1].Method == "refuse" || !strings.Contains(steps[1].TargetPath, "S03E09") {
		t.Fatalf("no clash: %+v", steps[1])
	}
}
