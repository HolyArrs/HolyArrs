package sources

import "testing"

// TestTextQuery_DropsApostrophes checks that apostrophes are dropped without a
// space (Schitt's -> Schitts) and other punctuation becomes a space.
func TestTextQuery_DropsApostrophes(t *testing.T) {
	for _, c := range []struct {
		in   Query
		want string
	}{
		{Query{Title: "Schitt's Creek"}, "Schitts Creek"},
		{Query{Title: "Schitt’s Creek"}, "Schitts Creek"},
		{Query{Title: "Grey's Anatomy"}, "Greys Anatomy"},
		{Query{Title: "Workin' Moms"}, "Workin Moms"},
		{Query{Title: "'Til Death"}, "Til Death"},
		{Query{Title: "Marvel's Agents of S.H.I.E.L.D."}, "Marvels Agents of S H I E L D"},
		{Query{Title: "Mission: Impossible", Year: 1996}, "Mission Impossible 1996"},
	} {
		if got := textQuery(c.in); got != c.want {
			t.Errorf("textQuery(%q) = %q, want %q", c.in.Title, got, c.want)
		}
	}
}
