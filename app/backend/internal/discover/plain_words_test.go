package discover

import (
	"slices"
	"strings"
	"testing"
)

func TestParse_Described(t *testing.T) {
	for _, c := range []struct {
		in       string
		plain    bool
		name     string
		kind     string
		from, to int
		genres   []string
		people   []string
		words    []string
	}{
		{in: "that space movie with the robot from 2014", kind: "movie", from: 2014, to: 2014, genres: []string{"Science Fiction"}, words: []string{"robot"}},
		{in: "2012", plain: true, name: "2012"},
		{in: "Se7en", plain: true, name: "Se7en"},
		{in: "Blade Runner 2049", plain: true, name: "Blade Runner 2049"},
		{in: "Space Jam", plain: true, name: "Space Jam"},
		{in: "", plain: true},
		{in: "   ", plain: true},
		{in: "scary movies from the 90s", kind: "movie", from: 1990, to: 1999, genres: []string{"Horror"}},
		{in: "funny shows 1980s", kind: "show", from: 1980, to: 1989, genres: []string{"Comedy"}},
		{in: "nineties sci-fi", from: 1990, to: 1999, genres: []string{"Science Fiction"}},
		{in: "horror", genres: []string{"Horror"}},
		{in: "scary comedy", genres: []string{"Horror", "Comedy"}},
		{in: "batman movie after 2010", name: "batman", kind: "movie", from: 2011},
		{in: "western films before 1970", kind: "movie", to: 1969, genres: []string{"Western"}},
		{in: "movie with Tom Hanks", kind: "movie", people: []string{"Tom Hanks"}},
		{in: "movie with tom hanks on an island", name: "island", kind: "movie", people: []string{"tom hanks"}},
		{in: "film starring keanu", kind: "movie", people: []string{"keanu"}},
		{in: "show about dragons", kind: "show", words: []string{"dragons"}},
		{in: "science fiction series in 2019", kind: "show", from: 2019, to: 2019, genres: []string{"Science Fiction"}},
		{in: "show me space movies", kind: "movie", genres: []string{"Science Fiction"}},
		{in: "Amélie film", name: "Amélie", kind: "movie"},
		{in: "映画 movie 2001", name: "映画", kind: "movie", from: 2001, to: 2001},
	} {
		q := ParsePlainWords(c.in)
		if q.Plain != c.plain || q.Name != strings.TrimSpace(c.name) || q.Kind != c.kind || q.YearFrom != c.from || q.YearTo != c.to {
			t.Errorf("%q: got %+v", c.in, q)
			continue
		}
		for _, x := range []struct {
			what      string
			got, want []string
		}{{"genres", q.Genres, c.genres}, {"people", q.People, c.people}, {"words", q.Words, c.words}} {
			if x.want == nil {
				x.want = []string{}
			}
			if !slices.Equal(x.got, x.want) {
				t.Errorf("%q %s: got %q want %q", c.in, x.what, x.got, x.want)
			}
		}
	}
}

func TestParse_OddInput(t *testing.T) {
	long := strings.Repeat("space movie with the robot ", 40)
	q := ParsePlainWords(long)
	if len([]rune(q.Text)) > maxRunes || q.Kind != "movie" {
		t.Fatalf("long input: %+v", q)
	}
	for _, s := range []string{"'", "-", "with", "from", "with the", "90s", "'s", "0000s", "with ́", "\x00\xff", "movie with -"} {
		q := ParsePlainWords(s)
		if q.Genres == nil || q.People == nil || q.Words == nil {
			t.Fatalf("%q: nil lists %+v", s, q)
		}
	}
}
