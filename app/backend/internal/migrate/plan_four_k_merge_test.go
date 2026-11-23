package migrate

import (
	"strings"
	"testing"
)

// radarrOneMovie is one Radarr read with one movie (tmdb 603) in its own
// library folder, on a profile with the given cutoff.
func radarrOneMovie(folder, cutoff string, monitored bool) *Snapshot {
	return &Snapshot{Kind: Radarr, Via: ViaAPI,
		Folders:         []Folder{{Path: folder}},
		QualityProfiles: []Profile{{ID: 1, Name: "Profile", Cutoff: cutoff}},
		Titles: []Title{{SourceID: 1, Kind: "movie", Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"},
			Path: folder + "/The Matrix (1999)", Monitored: monitored, ProfileID: 1}}}
}

// TestPlanMergesFourKRadarrCopy: a movie in a 1080p and a 4K Radarr becomes
// one title whose main plan is the 1080p copy and whose FourK plan is the
// 4K app's folder, whichever read comes first; nothing is listed as not
// moved for it.
func TestPlanMergesFourKRadarrCopy(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", false)
	for _, in := range [][]Input{{{1, hd}, {2, uhd}}, {{1, uhd}, {2, hd}}} {
		p := BuildPlan(in, Choices{})
		titles := p.Group(GroupTitles).Items
		if len(titles) != 1 {
			t.Fatalf("%d title items, want 1: %+v", len(titles), titles)
		}
		it := titles[0]
		tp := it.Title
		if tp.FourK == nil || tp.Library != "/movies" || tp.Folder != "The Matrix (1999)" || tp.Preset != "Standard" || !tp.KeepUpToDate {
			t.Fatalf("main plan %+v", tp)
		}
		fk := tp.FourK
		if fk.Library != "/movies4k" || fk.Folder != "The Matrix (1999)" || fk.Preset != "4K" || fk.KeepUpToDate || fk.Unresolved != "" || fk.From != "radarr titles[0]" {
			t.Fatalf("4K plan %+v", fk)
		}
		if it.Name != "The Matrix (1999)" || !strings.Contains(it.Detail, "its 4K copy uses the folder /movies4k/The Matrix (1999) where it is") {
			t.Fatalf("item %q detail %q", it.Name, it.Detail)
		}
		for _, n := range p.NotMoved {
			if n.Group == GroupTitles || strings.Contains(n.Why, "imported once") {
				t.Fatalf("the merged movie is listed: %+v", n)
			}
		}
	}
}

// TestPlanSameKindRadarrsImportOnce: two 1080p Radarrs, or two 4K ones,
// keep today's "imported once" line for the second copy.
func TestPlanSameKindRadarrsImportOnce(t *testing.T) {
	for _, cutoffs := range [][2]string{{"Bluray-1080p", "WEBDL-1080p"}, {"Remux-2160p", "WEBDL-2160p"}} {
		a := radarrOneMovie("/movies", cutoffs[0], true)
		b := radarrOneMovie("/movies2", cutoffs[1], true)
		p := BuildPlan([]Input{{1, a}, {2, b}}, Choices{})
		titles := p.Group(GroupTitles).Items
		if len(titles) != 1 || titles[0].Title.FourK != nil || titles[0].Title.Library != "/movies" {
			t.Fatalf("cutoffs %v: %+v", cutoffs, titles)
		}
		if n, ok := notMoved(p, "radarr titles[0]"); !ok || !strings.Contains(n.Why, "imported once") {
			t.Fatalf("cutoffs %v: second copy %+v", cutoffs, n)
		}
	}
}

// TestPlanSonarrPairKeepsImportedOnce: shows keep today's behaviour even
// when one Sonarr's profile aims for 2160p.
func TestPlanSonarrPairKeepsImportedOnce(t *testing.T) {
	show := func(folder, cutoff string) *Snapshot {
		return &Snapshot{Kind: Sonarr, Via: ViaAPI, Folders: []Folder{{Path: folder}},
			QualityProfiles: []Profile{{ID: 1, Name: "Profile", Cutoff: cutoff}},
			Titles:          []Title{{SourceID: 1, Kind: "show", Name: "Lost", IDs: map[string]string{"tvdb": "73739"}, Path: folder + "/Lost", Monitored: true, ProfileID: 1}}}
	}
	p := BuildPlan([]Input{{1, show("/tv", "Bluray-1080p")}, {2, show("/tv4k", "Remux-2160p")}}, Choices{})
	titles := p.Group(GroupTitles).Items
	if len(titles) != 1 || titles[0].Title.FourK != nil || titles[0].Title.Library != "/tv" {
		t.Fatalf("shows %+v", titles)
	}
	if n, ok := notMoved(p, "sonarr titles[0]"); !ok || !strings.Contains(n.Why, "imported once") {
		t.Fatalf("second show %+v", n)
	}
}

// TestPlanMergesFourKCopyWithImportedRules: the 4K kind comes from the
// cutoff even when the profile becomes its own Pick rules 1:1.
func TestPlanMergesFourKCopyWithImportedRules(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	uhd.QualityProfiles[0].Name = "Ultra-HD"
	uhd.QualityProfiles[0].Scores = map[string]int{"x265": 10}
	uhd.CustomFormats = []Named{{ID: 1, Name: "x265"}}
	p := BuildPlan([]Input{{1, hd}, {2, uhd}}, Choices{})
	if r := item(t, p, GroupRules, "pick_rules:2:radarr:1").Rules; r.Body == nil {
		t.Fatalf("the 4K profile did not become its own Pick rules: %+v", r)
	}
	it := item(t, p, GroupTitles, "title:movie:tmdb:603")
	if it.Title.FourK == nil || it.Title.FourK.Preset != "Ultra-HD" || it.Title.FourK.Library != "/movies4k" {
		t.Fatalf("merged plan %+v", it.Title)
	}
}

// TestPlanMergesUnplacedFourKCopy: an unplaced 4K copy rides along with
// its reason instead of a not-moved line; an unplaced 1080p copy keeps
// today's not-placed line and leaves the 4K title as it was.
func TestPlanMergesUnplacedFourKCopy(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	uhd.Titles[0].Path = "/elsewhere/The Matrix (1999)"
	p := BuildPlan([]Input{{1, hd}, {2, uhd}}, Choices{})
	it := item(t, p, GroupTitles, "title:movie:tmdb:603")
	fk := it.Title.FourK
	if fk == nil || fk.Unresolved == "" || !strings.Contains(it.Detail, "its 4K copy is not placed:") {
		t.Fatalf("unplaced 4K copy %+v detail %q", fk, it.Detail)
	}
	for _, n := range p.NotMoved {
		if n.Group == GroupTitles {
			t.Fatalf("the merged movie is listed: %+v", n)
		}
	}
	// The other order: the 1080p copy cannot be placed, so it stays behind.
	hd.Titles[0].Path = "/elsewhere/The Matrix (1999)"
	uhd.Titles[0].Path = "/movies4k/The Matrix (1999)"
	p = BuildPlan([]Input{{1, uhd}, {2, hd}}, Choices{})
	it = item(t, p, GroupTitles, "title:movie:tmdb:603")
	if it.Title.FourK != nil || it.Title.Library != "/movies4k" {
		t.Fatalf("4K-only title %+v", it.Title)
	}
	if n, ok := notMoved(p, "radarr titles[0]"); !ok || !strings.Contains(n.Why, "not placed") {
		t.Fatalf("unplaced 1080p copy %+v", n)
	}
}
