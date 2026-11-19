package shows

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type tvSearchFixture struct {
	*fakeShows
	results  map[string][]metadata.Show
	searches []string
	fail     error
}

func (p *tvSearchFixture) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	p.searches = append(p.searches, q)
	return p.results[q], p.fail
}

func TestTVReleaseSearchNames(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"[Anime Time] Baki Hanma Season 02 (Part 1) [NF] [Dual Audio] [1080p]", "Baki Hanma"},
		{"[AnimeKaizoku] One Punch Man Season 1 720p 10bit HEVC Dual Audio [Lucy]", "One Punch Man"},
		{"Dragon Ball Super Complete Season 2 Golden Frieza", "Dragon Ball Super"},
		{"Dragon.Ball.Super.EP001-131.1080p.ENG.SPA.JAP.x264.MaDHeX.[MaDTiA_]", "Dragon Ball Super"},
		{"Subs, Brooklyn.Nine-Nine.S05E01.1080p.BluRay.x265-RARBG.mp4, Brooklyn.Nine-Nine.S05E02.mkv", "Brooklyn Nine-Nine"},
		{"The.Office.US.S09.BDRip.x264-FGT", "The Office US"},
		{"Law and Order - SVU Season 03", "Law and Order - SVU"},
		{"[Group] Imaginary.Show.EP001-099.720p", "Imaginary Show"},
		{"Complete Savages", "Complete Savages"},
		{"Episodes", "Episodes"},
		{"Love, Death and Robots", "Love, Death and Robots"},
		{"Love, Death and Robots S01E01.mkv", "Love, Death and Robots"},
		{"[Group] [Dual Audio] Imaginary.Show.Season.1", "Imaginary Show"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, _ := showFolderName(tc.raw)
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTVProviderBackedSuggestions(t *testing.T) {
	for _, tc := range []struct {
		name, canonical string
		year            int
	}{
		{"Law and Order - SVU", "Law & Order: Special Victims Unit", 1999},
		{"MS Gundam Wing", "Mobile Suit Gundam Wing", 1995},
		{"MSG - The 08th MS Team", "Mobile Suit Gundam: The 08th MS Team", 1996},
		{"The Office US", "The Office", 2005},
		{"AB Space Adventures", "Alpha Beta Space Adventures", 2001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := metadata.Show{Name: tc.canonical, Year: tc.year}
			score := tvNameScore(tc.name, tc.year, s)
			if score < library.MatchUnsure || score >= library.MatchSure {
				t.Errorf("suggestion score %v", score)
			}
			if tvNameScore(tc.name, tc.year+20, s) >= score {
				t.Error("lost year penalty")
			}
		})
	}
	for _, tc := range []struct{ a, b string }{
		{"MS", "Mobile Suit"}, {"MS Gundam Wing", "Mystery Space Gundam Wars"},
		{"ms Gundam Wing", "Mobile Suit Gundam Wing"}, {"ABC Show", "Alpha Beta Charlie Other Show"},
	} {
		if tvAcronymMatch(tc.a, tc.b) {
			t.Errorf("unsafe expansion %q -> %q", tc.a, tc.b)
		}
	}
	alias := metadata.Show{Name: "Canonical Name", OtherNames: []string{"Alternate Name"}, Year: 2020}
	if tvNameScore("Alternate Name", 2020, alias) != 1 {
		t.Error("provider alias ignored")
	}
}

func TestTVSearchFallbackCacheAndBudget(t *testing.T) {
	p := &tvSearchFixture{fakeShows: &fakeShows{}, results: map[string][]metadata.Show{"Gundam Wing": {{Name: "Mobile Suit Gundam Wing", Year: 1995, Source: "tmdb", IDs: map[string]string{"tmdb": "90"}}}}}
	m := newTVMatcher(p)
	for _, folder := range []string{"MS Gundam Wing (1995) Season 1", "MS Gundam Wing (1995) Season 2"} {
		_, _, got, err := m.match(context.Background(), folder)
		if err != nil || len(got) != 1 || got[0].Score < library.MatchUnsure {
			t.Fatalf("%s: %+v %v", folder, got, err)
		}
	}
	if len(p.searches) != 2 {
		t.Fatalf("season searches not cached: %v", p.searches)
	}
	m.remaining = 1
	for _, q := range []string{"first", "second"} {
		if _, err := m.search(context.Background(), q, true); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.searches) != 3 {
		t.Fatal("fallback budget exceeded")
	}
	if _, err := m.search(context.Background(), "normal", false); err != nil || len(p.searches) != 4 {
		t.Fatal("budget suppressed normal lookup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.search(ctx, "normal", false); !errors.Is(err, context.Canceled) {
		t.Fatal("cached lookup ignored cancellation")
	}
	p.fail = fmt.Errorf("offline")
	for range 2 {
		if _, err := m.search(context.Background(), "retry", false); err == nil {
			t.Fatal("error hidden")
		}
	}
	if len(p.searches) != 6 {
		t.Fatal("transient error cached")
	}
}

func TestTVPackMatchingPlan(t *testing.T) {
	svc, store, _, _ := addShowService(t)
	lf := addShowFolder(t, store)
	p := &tvSearchFixture{fakeShows: &fakeShows{}, results: map[string][]metadata.Show{}}
	cases := []struct {
		folder, query, name string
		year                int
	}{
		{"Law and Order - SVU Season 03", "Law and Order - SVU", "Law & Order: Special Victims Unit", 1999},
		{"The.Office.US.S02.BRRip.x264-ION10", "The Office US", "The Office", 2005},
		{"Dragon Ball Super Complete Season 2 Golden Frieza", "Dragon Ball Super", "Dragon Ball Super", 2015},
		{"Subs, Brooklyn.Nine-Nine.S05E01.1080p.BluRay.mp4, Brooklyn.Nine-Nine.S05E02.mkv", "Brooklyn Nine-Nine", "Brooklyn Nine-Nine", 2013},
		{"Animation Pack/1a. MS Gundam Wing (1995-96)", "Gundam Wing", "Mobile Suit Gundam Wing", 1995},
		{"Animation Pack/2a. MSG - The 08th MS Team (1996-99)", "The 08th MS Team", "Mobile Suit Gundam: The 08th MS Team", 1996},
	}
	for i, tc := range cases {
		sh := labShow("tmdb", fmt.Sprint(i+1000))
		sh.Name, sh.Year = tc.name, tc.year
		p.set(sh, showID(sh))
		p.results[tc.query] = []metadata.Show{sh}
		file := "Season 1/Show.S01E01.mkv"
		if i == 4 {
			file = "Extras/Show.S01E01.mkv"
		}
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(tc.folder), filepath.FromSlash(file)), "video")
	}
	svc.Shows = metadata.ShowSources{"tmdb": p}
	plan, err := PlanShowAdoptFrom(context.Background(), store.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	rows := byFolder(plan)
	for i, tc := range cases {
		c, ok := rows[tc.folder]
		if !ok || c.Match == nil || c.Match.ID != fmt.Sprintf("tmdb:%d", 1000+i) {
			t.Errorf("%s: %+v", tc.folder, c)
			continue
		}
		if i == 4 {
			if c.Status != "no_video" || c.Episodes != 0 {
				t.Errorf("extras claimed: %+v", c)
			}
		} else if c.Status != "unsure" || c.Episodes != 1 {
			t.Errorf("unusable suggestion: %+v", c)
		}
	}
	if count(t, store, "titles") != 0 {
		t.Fatal("planning wrote titles")
	}
}

func TestTVSeasonEpisodeEvidence(t *testing.T) {
	vs := []library.AdoptVideo{{Path: "Season 4/Dragon Ball Super - 64 - Episode.mkv"}, {Path: "Season 4/Dragon Ball Super - 65 - Episode.mkv"}}
	p := &tvSearchFixture{fakeShows: &fakeShows{}, results: map[string][]metadata.Show{"Dragon Ball Super": {{Name: "Dragon Ball Super", Year: 2015, Source: "tmdb", IDs: map[string]string{"tmdb": "62715"}}}}}
	m := newTVMatcher(p)
	n, _, got, err := m.matchFolder(context.Background(), "Season 4 - 'Future' Trunks Saga [1080p]", vs)
	if err != nil || n != "Dragon Ball Super" || len(got) != 1 || got[0].Score != 0.85 {
		t.Fatalf("episode evidence: %q %+v %v", n, got, err)
	}
	if len(p.searches) != 2 {
		t.Fatalf("unexpected requests: %v", p.searches)
	}
	for _, paths := range [][]string{
		{"Season 1/Example.S01E01.mkv"},
		{"Season 1/Example.S01E01.mkv", "Season 1/Other.S01E02.mkv"},
		{"Season 1/Extras/Example.S01E01.mkv", "Season 1/Extras/Example.S01E02.mkv"},
		{"Season 1/Example.S01E01.mkv", "Season 1/Example.S01E02.mkv", "Season 1/Unknown.mkv"},
	} {
		var files []library.AdoptVideo
		for _, p := range paths {
			files = append(files, library.AdoptVideo{Path: p})
		}
		if tvEpisodeName(files) != "" {
			t.Errorf("unsafe evidence: %v", paths)
		}
	}
	m = newTVMatcher(p)
	m.remaining = 0
	_, _, got, err = m.matchFolder(context.Background(), "Season 4", vs)
	if err != nil || len(got) != 0 {
		t.Fatalf("budget bypassed: %+v %v", got, err)
	}
}
