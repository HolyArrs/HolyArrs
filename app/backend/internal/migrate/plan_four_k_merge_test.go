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
			if n.Group == GroupTitles || strings.HasPrefix(n.From, "radarr titles") {
				t.Fatalf("the merged movie is listed: %+v", n)
			}
		}
	}
}

// TestPlanFourKCopySharingTheFolderIsNotPlaced: when both Radarrs keep the
// movie in one folder (a 1080p and a 2160p file side by side), the plan
// says the 4K copy is not placed, in either order, instead of promising a
// 4K Version the real run cannot add; the main copy stays the 1080p one.
func TestPlanFourKCopySharingTheFolderIsNotPlaced(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	uhd := radarrOneMovie("/movies", "Remux-2160p", true)
	uhd.Titles[0].Path = "/movies/the matrix (1999)" // the same folder, told by case only
	for _, in := range [][]Input{{{1, hd}, {2, uhd}}, {{1, uhd}, {2, hd}}} {
		p := BuildPlan(in, Choices{})
		it := item(t, p, GroupTitles, "title:movie:tmdb:603")
		tp := it.Title
		if tp.FourK == nil || tp.Preset != "Standard" || !strings.EqualFold(tp.Folder, "The Matrix (1999)") {
			t.Fatalf("main plan %+v", tp)
		}
		if !strings.Contains(tp.FourK.Unresolved, "shares the folder") || !strings.Contains(it.Detail, "its 4K copy is not placed: it shares the folder") {
			t.Fatalf("4K plan %+v detail %q", tp.FourK, it.Detail)
		}
		for _, n := range p.NotMoved {
			if n.Group == GroupTitles {
				t.Fatalf("the merged movie is listed: %+v", n)
			}
		}
	}
}

// TestPlanFourKCopyInAnotherTitlesFolderIsNotPlaced: a merged movie's 4K
// folder that another planned movie uses as its own folder (told by case
// only) is not placed, in either read order, since the real run could not
// add it; the other movie keeps its folder.
func TestPlanFourKCopyInAnotherTitlesFolderIsNotPlaced(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	hd.Folders = append(hd.Folders, Folder{Path: "/movies4k"})
	hd.Titles = append(hd.Titles, Title{SourceID: 2, Kind: "movie", Name: "Other", Year: 2000, IDs: map[string]string{"tmdb": "999"},
		Path: "/movies4k/the matrix (1999)", Monitored: true, ProfileID: 1})
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	for _, in := range [][]Input{{{1, hd}, {2, uhd}}, {{1, uhd}, {2, hd}}} {
		p := BuildPlan(in, Choices{})
		it := item(t, p, GroupTitles, "title:movie:tmdb:603")
		fk := it.Title.FourK
		if fk == nil || !strings.Contains(fk.Unresolved, "is also used by Other (2000)") ||
			!strings.Contains(it.Detail, "its 4K copy is not placed: its folder /movies4k/The Matrix (1999) is also used by Other (2000)") ||
			strings.Contains(it.Detail, "its 4K copy uses the folder") {
			t.Fatalf("4K plan %+v detail %q", fk, it.Detail)
		}
		other := item(t, p, GroupTitles, "title:movie:tmdb:999")
		if other.Title.Unresolved != "" || other.Title.Library != "/movies4k" || other.Title.Folder != "the matrix (1999)" {
			t.Fatalf("the other movie's plan %+v", other.Title)
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

// TestPlanFourKCopyInAFolderAVersionCannotKeep: a 4K copy whose folder name
// starts with a space cannot be a Version's folder, as adding the Version
// checks, so the plan says the 4K copy is not placed, in either read order,
// instead of promising it.
func TestPlanFourKCopyInAFolderAVersionCannotKeep(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	uhd.Titles[0].Path = "/movies4k/ The Matrix (1999)"
	for _, in := range [][]Input{{{1, hd}, {2, uhd}}, {{1, uhd}, {2, hd}}} {
		p := BuildPlan(in, Choices{})
		it := item(t, p, GroupTitles, "title:movie:tmdb:603")
		if it.Title.Folder != "The Matrix (1999)" || it.Title.FourK == nil {
			t.Fatalf("main plan %+v", it.Title)
		}
		if !strings.Contains(it.Detail, `its 4K copy is not placed: its folder name " The Matrix (1999)" cannot be a Version's folder`) {
			t.Fatalf("detail %q", it.Detail)
		}
	}
}

// TestPlanFolderCaseIsAToZOnly: folders are matched as adding a Version
// matches them, letters A to Z in either case and every other letter as it
// is, so "Ämélie (2001)" and "ämélie (2001)" are two folders and the 4K
// copy is placed, while "AMÉLIE (2001)" and "amÉlie (2001)" are one.
func TestPlanFolderCaseIsAToZOnly(t *testing.T) {
	for _, c := range []struct {
		main, fourK string
		shared      bool
	}{{"Ämélie (2001)", "ämélie (2001)", false}, {"AMÉLIE (2001)", "amÉlie (2001)", true}} {
		hd := radarrOneMovie("/movies", "Bluray-1080p", true)
		hd.Titles[0].Path = "/movies/" + c.main
		uhd := radarrOneMovie("/movies", "Remux-2160p", true)
		uhd.Titles[0].Path = "/movies/" + c.fourK
		fk := item(t, BuildPlan([]Input{{1, hd}, {2, uhd}}, Choices{}), GroupTitles, "title:movie:tmdb:603").Title.FourK
		if fk == nil || strings.Contains(fk.Unresolved, "shares the folder") != c.shared {
			t.Fatalf("%q and %q: 4K plan %+v, want shared %v", c.main, c.fourK, fk, c.shared)
		}
	}
}

// TestPlanUnplacedFourKCopyReadFirst: a 4K copy that cannot be placed,
// read before the 1080p copy, still rides along with the 1080p title with
// its reason, as when it is read second, instead of a not-moved line; so
// the main copy keeps the merged movie's rules for its file.
func TestPlanUnplacedFourKCopyReadFirst(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	uhd.Titles[0].Path = "/elsewhere/The Matrix (1999)"
	for _, in := range [][]Input{{{1, uhd}, {2, hd}}, {{1, hd}, {2, uhd}}} {
		p := BuildPlan(in, Choices{})
		it := item(t, p, GroupTitles, "title:movie:tmdb:603")
		if it.Title.Library != "/movies" || it.Title.FourK == nil || it.Title.FourK.Unresolved == "" || !strings.Contains(it.Detail, "its 4K copy is not placed:") {
			t.Fatalf("plan %+v detail %q", it.Title, it.Detail)
		}
		for _, n := range p.NotMoved {
			if n.Group == GroupTitles {
				t.Fatalf("the merged movie is listed: %+v", n)
			}
		}
	}
}

// TestPlanUnplacedFourKCopyKeepsOtherLines: carrying an unplaced 4K copy
// takes away its own not-moved line only, not another movie's line with
// the same from (both apps' first title is "radarr titles[0]").
func TestPlanUnplacedFourKCopyKeepsOtherLines(t *testing.T) {
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	uhd.Titles[0].Path = "/elsewhere/The Matrix (1999)"
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	hd.Titles[0].SourceID = 2
	hd.Titles = append([]Title{{SourceID: 1, Kind: "movie", Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"},
		Path: "/elsewhere/Heat (1995)", Monitored: true, ProfileID: 1}}, hd.Titles...)
	p := BuildPlan([]Input{{1, uhd}, {2, hd}}, Choices{})
	if it := item(t, p, GroupTitles, "title:movie:tmdb:603"); it.Title.FourK == nil || it.Title.FourK.Unresolved == "" {
		t.Fatalf("the 4K copy was not carried: %+v", it.Title)
	}
	var lines []string
	for _, n := range p.NotMoved {
		if n.Group == GroupTitles {
			lines = append(lines, n.From+" "+n.What)
		}
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "Heat") {
		t.Fatalf("not-moved title lines %q, want only Heat's", lines)
	}
}

// TestSameFileName: where the file system ignores case, a recorded name in
// another case is the same file; where it does not, it is another file.
func TestSameFileName(t *testing.T) {
	if !sameFileName(true, "Movie.2160p.mkv", "MOVIE.2160P.MKV") || sameFileName(false, "Movie.2160p.mkv", "MOVIE.2160P.MKV") || !sameFileName(false, "a.mkv", "a.mkv") {
		t.Fatal("file names compared without the file system's case rule")
	}
}

// TestPlanFileOnlyForMergedMovies: the recorded file name stays in the
// plan only for a merged movie, so an ordinary movie's plan, and the hash
// an approved import was checked against, are as before.
func TestPlanFileOnlyForMergedMovies(t *testing.T) {
	hd := radarrOneMovie("/movies", "Bluray-1080p", true)
	hd.Titles[0].Files = []TitleFile{{Name: "The.Matrix.1999.1080p.mkv", Quality: "Bluray-1080p"}}
	if it := item(t, BuildPlan([]Input{{1, hd}}, Choices{}), GroupTitles, "title:movie:tmdb:603"); it.Title.File != "" {
		t.Fatalf("an ordinary movie's plan has its file %q", it.Title.File)
	}
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	uhd.Titles[0].Files = []TitleFile{{Name: "The.Matrix.1999.2160p.mkv", Quality: "Remux-2160p"}}
	for _, in := range [][]Input{{{1, hd}, {2, uhd}}, {{1, uhd}, {2, hd}}} {
		it := item(t, BuildPlan(in, Choices{}), GroupTitles, "title:movie:tmdb:603")
		if it.Title.File != "The.Matrix.1999.1080p.mkv" || it.Title.FourK == nil || it.Title.FourK.File != "The.Matrix.1999.2160p.mkv" {
			t.Fatalf("merged plan %+v", it.Title)
		}
	}
}
