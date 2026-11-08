package downloads

import (
	"strings"
	"testing"
)

func TestGuessOf(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, cat string
		kind      string
		title     string
		year      int
		covers    string
	}{
		{"Paper.Harbour.2023.1080p.WEB-DL.H264-GRP", "movies", "movie", "Paper Harbour", 2023, ""},
		{"Some.Show.S01E02.720p.HDTV.x264-GRP", "tv_shows", "show", "Some Show", 0, "S01E02"},
		{"Some.Show.S02.1080p.WEB-DL-GRP", "tv_shows", "show", "Some Show", 0, "Season 2"},
		{"Some.Show.S01E02E03.1080p.WEB-GRP", "", "show", "Some Show", 0, "S01E02-E03"},
		{"setup.exe", "", "", "", 0, ""},
	}
	for _, c := range cases {
		g := guessOf(c.name, c.cat)
		if g.kind != c.kind || (c.kind != "" && (!strings.EqualFold(g.title, c.title) || g.year != c.year)) {
			t.Errorf("%s: %+v", c.name, g)
		}
		if c.kind == "show" && coversLine(g) != c.covers {
			t.Errorf("%s covers %q", c.name, coversLine(g))
		}
	}
	g := guessOf("Some.Show.S02.1080p.WEB-DL-GRP", "tv")
	if !coversEpisode(g, 2, 5) || coversEpisode(g, 1, 5) {
		t.Fatalf("season pack covers %+v", g)
	}
}

func TestPickSure(t *testing.T) {
	t.Parallel()
	one := []Candidate{{ID: "tmdb:1", Name: "Paper Harbour", Year: 2023, Score: likeness("Paper Harbour", "Paper Harbour")}, {ID: "tmdb:2", Name: "Paper Moon", Year: 1973, Score: likeness("Paper Harbour", "Paper Moon")}}
	if i := pickSure(one, 2023); i != 0 {
		t.Fatalf("one exact: %d", i)
	}
	if i := pickSure(one, 1990); i != -1 {
		t.Fatalf("wrong year must be unsure: %d", i)
	}
	twins := []Candidate{{ID: "tmdb:1", Name: "Quiet Orchard", Year: 2022, Score: 100}, {ID: "tmdb:9", Name: "Quiet Orchard", Year: 1999, Score: 100}}
	if i := pickSure(twins, 0); i != -1 {
		t.Fatalf("two alike without a year must be unsure: %d", i)
	}
	if i := pickSure(twins, 1999); i != 1 {
		t.Fatalf("the year decides: %d", i)
	}
	if likeness("The Office", "Office") != 100 || likeness("Paper Harbour", "Lighthouse") > 50 {
		t.Fatal("likeness")
	}
}

// An indexer-manager category like "prowlarr" says nothing about the kind: a
// name with a year and a resolution is still a movie; other names stay out.
func TestGuessNeutralCategory(t *testing.T) {
	t.Parallel()
	if g := guessOf("Root_Cause_2019_Bluray-1080p", "prowlarr"); g.kind != "movie" {
		t.Fatalf("prowlarr movie: %+v", g)
	}
	if g := guessOf("Some.Linux.Distro.x64", "linux"); g.kind != "" {
		t.Fatalf("linux: %+v", g)
	}
}
