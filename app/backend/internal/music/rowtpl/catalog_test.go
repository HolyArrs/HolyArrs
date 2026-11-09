package rowtpl

import (
	"regexp"
	"strings"
	"testing"
)

var brandWords = []string{"spotify", "apple", "itunes", "deezer", "listenbrainz", "musicbrainz", "youtube", "billboard", "shazam", "tidal", "amazon", "pandora", "soundcloud", "last.fm", "rapidapi", "kworb",
	"essentials", "around the world", "next up", "fresh finds", "this is", "daily mix", "made for you", "on repeat", "hits by decade", "today's top hits", "rap life"}

var slotRe = regexp.MustCompile(`\{[a-z0-9]+\}`)

var allowedSlots = map[string]bool{"{genre}": true, "{genre2}": true, "{substyle}": true, "{mood}": true, "{season}": true, "{decade}": true, "{name}": true, "{for}": true, "{activity}": true, "{basis}": true, "{year}": true, "{n}": true, "{country}": true, "{place}": true, "{neighbor}": true, "{area}": true, "{artist}": true, "{mix}": true, "{artists}": true, "{artist2}": true}

func TestCatalog_Lint(t *testing.T) {
	if len(Catalog) < 150 {
		t.Fatalf("catalog has %d templates, want at least 150", len(Catalog))
	}
	ids := map[string]bool{}
	sig := map[string]string{}
	for _, tp := range Catalog {
		if ids[tp.ID] {
			t.Errorf("duplicate id %s", tp.ID)
		}
		ids[tp.ID] = true
		if tp.Title == "" || tp.Reason == "" || tp.Src == "" || tp.Sort == "" || tp.Ref == "" || tp.Pages == "" || tp.Fam == "" || tp.Prior <= 0 {
			t.Errorf("%s: missing field", tp.ID)
		}
		if strings.Contains(strings.ToLower(tp.Src), "v7") || strings.Contains(strings.ToLower(tp.Sort), "v7") {
			t.Errorf("%s: source or sort names an engine version", tp.ID)
		}
		if strings.Trim(tp.Pages, "ENGMSDA") != "" {
			t.Errorf("%s: bad pages %q", tp.ID, tp.Pages)
		}
		if _, ok := familyMax[tp.Fam]; !ok {
			t.Errorf("%s: unknown family %s", tp.ID, tp.Fam)
		}
		if tp.Fill == nil && tp.Need != "v7" && tp.Need != "cal" {
			t.Errorf("%s: no fill", tp.ID)
		}
		for _, s := range append([]string{tp.Title, tp.Reason}, tp.Vars...) {
			for _, w := range brandWords {
				if strings.Contains(strings.ToLower(s), w) {
					t.Errorf("%s: banned word %q in %q", tp.ID, w, s)
				}
			}
			for _, m := range slotRe.FindAllString(s, -1) {
				if !allowedSlots[m] {
					t.Errorf("%s: unknown slot %s", tp.ID, m)
				}
			}
			if strings.Contains(s, "!") {
				t.Errorf("%s: exclamation mark", tp.ID)
			}
		}
		k := tp.Src + "|" + tp.Sort + "|" + tp.Filter + "|" + tp.Pages
		if o, dup := sig[k]; dup {
			t.Errorf("%s and %s have the same source, sort, filters and pages", o, tp.ID)
		}
		sig[k] = tp.ID
	}
}

func TestCatalog_TitlesFitLongestNames(t *testing.T) {
	long := map[string]string{"{genre}": "Soul & funk", "{genre2}": "Soul & funk", "{substyle}": "Alternative hip-hop", "{mood}": "High-energy", "{season}": "Día de los Muertos",
		"{decade}": "60s", "{name}": "Día de los Muertos", "{for}": "for a night in", "{activity}": "chill at home", "{year}": "2026", "{n}": "50",
		"{country}": "Dominican Rep.", "{place}": "in Dominican Rep.", "{neighbor}": "in Dominican Rep.", "{area}": "Asia-Pacific"}
	for _, tp := range Catalog {
		ok := false
		for _, s := range append([]string{tp.Title}, tp.Vars...) {
			if strings.Contains(s, "{artist}") || strings.Contains(s, "{mix}") {
				ok = true
				break
			}
			x := s
			for k, v := range long {
				x = strings.ReplaceAll(x, k, v)
			}
			if len([]rune(x)) <= 40 {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%s: no title fits 40 characters with the longest names", tp.ID)
		}
	}
}

func TestCatalog_CoversEveryPageKind(t *testing.T) {
	n := map[byte]int{}
	for _, tp := range Catalog {
		for i := 0; i < len(tp.Pages); i++ {
			n[tp.Pages[i]]++
		}
	}
	for _, p := range []byte("ENGMSDA") {
		if n[p] < 10 {
			t.Errorf("page %c has %d templates", p, n[p])
		}
	}
}

// TestCatalog_NoGenreJumpRow checks the "If you like {genre}, try {genre2}" row stays gone. The catalog has no listener overlap between genres, so it showed albums far from the page.
func TestCatalog_NoGenreJumpRow(t *testing.T) {
	for _, tp := range Catalog {
		if strings.Contains(tp.Title, "try {genre2}") {
			t.Fatalf("%s %q still in the catalog", tp.ID, tp.Title)
		}
	}
}
