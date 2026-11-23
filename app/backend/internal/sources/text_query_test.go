package sources

import "testing"

// TestTextQuery_DropsApostrophes checks that apostrophes and dots are dropped
// without a space (Schitt's -> Schitts, S.W.A.T. -> SWAT), as the older apps
// send them, and other punctuation, a star too, becomes a space.
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
		{Query{Title: "Marvel's Agents of S.H.I.E.L.D."}, "Marvels Agents of SHIELD"},
		{Query{Title: "Mission: Impossible", Year: 1996}, "Mission Impossible 1996"},
		{Query{Title: "S.W.A.T."}, "SWAT"},
		{Query{Title: "S.W.A.T.", Year: 2003}, "SWAT 2003"},
		{Query{Title: "M*A*S*H"}, "M A S H"},
		{Query{Title: "Agents of S.H.I.E.L.D."}, "Agents of SHIELD"},
		{Query{Title: "E.T. the Extra-Terrestrial", Year: 1982}, "ET the Extra Terrestrial 1982"},
		{Query{Title: "U.S. Marshals"}, "US Marshals"},
		{Query{Title: "L.A. Confidential"}, "LA Confidential"},
		{Query{Title: "A Man Called Otto"}, "A Man Called Otto"},
		{Query{Title: "I Am Legend"}, "I Am Legend"},
		{Query{Title: "V"}, "V"},
		{Query{Title: "X-Men"}, "X Men"},
		{Query{Title: "9-1-1"}, "9 1 1"},
		{Query{Title: "Mr. Robot"}, "Mr Robot"},
		{Query{Title: "Dr. Stone"}, "Dr Stone"},
		{Query{Title: "The A-Team"}, "The A Team"},
		{Query{Title: "P.S. I Love You", Year: 2007}, "PS I Love You 2007"},
		// only a dot between single letters goes; any other dot is a gap
		{Query{Title: "2.0"}, "2 0"},
		{Query{Title: "Robot 2.0"}, "Robot 2 0"},
		{Query{Title: "Jackass 2.5"}, "Jackass 2 5"},
		{Query{Title: "Jackass 3.5"}, "Jackass 3 5"},
		{Query{Title: "Evangelion: 1.0 You Are (Not) Alone"}, "Evangelion 1 0 You Are Not Alone"},
		{Query{Title: "Evangelion: 3.0+1.0 Thrice Upon a Time"}, "Evangelion 3 0 1 0 Thrice Upon a Time"},
		{Query{Title: "10.0 Earthquake"}, "10 0 Earthquake"},
		{Query{Title: "1.5 Kilo"}, "1 5 Kilo"},
		{Query{Title: "Web 3.0"}, "Web 3 0"},
		{Query{Title: "D.Gray-man"}, "D Gray man"},
		{Query{Title: "Dr.STONE"}, "Dr STONE"},
		{Query{Title: "Mr.Robot"}, "Mr Robot"},
		{Query{Title: "tick, tick...BOOM!"}, "tick tick BOOM"},
		{Query{Title: "House M.D."}, "House MD"},
		{Query{Title: "G.I. Joe"}, "GI Joe"},
		{Query{Title: "The Man from U.N.C.L.E."}, "The Man from UNCLE"},
		{Query{Title: "F.R.I.E.N.D.S"}, "FRIENDS"},
		{Query{Title: "N.C.I.S."}, "NCIS"},
		{Query{Title: "R.I.P.D."}, "RIPD"},
		{Query{Title: "A.P. Bio"}, "AP Bio"},
	} {
		if got := textQuery(c.in); got != c.want {
			t.Errorf("textQuery(%q) = %q, want %q", c.in.Title, got, c.want)
		}
	}
}

// No title, however marked up, goes out as empty text.
func TestTextQueryNeverEmpty(t *testing.T) {
	for _, s := range []string{"2.0", "...", "S.W.A.T.", "M*A*S*H", "F.R.I.E.N.D.S", "R.I.P.D.", "tick, tick...BOOM!", "I.T.", "E.R.", "V", "9-1-1", "'71", "W.", "A.", "1.0"} {
		if textQuery(Query{Title: s}) == "" && s != "..." {
			t.Errorf("textQuery(%q) is empty", s)
		}
	}
}

// Music names go out as the music app sends them: a dot is a gap.
func TestMusicText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"R.E.M. Automatic for the People", "R E M Automatic for the People"},
		{"AC/DC Back in Black", "AC DC Back in Black"},
		{"Destiny's Child Survivor", "Destinys Child Survivor"},
	} {
		if got := musicText(c.in); got != c.want {
			t.Errorf("musicText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The id-answer check reads a title and a release name the same way, however
// each writes letters apart.
func TestNamesAllLettersApart(t *testing.T) {
	for _, c := range []struct {
		title, release string
		want           bool
	}{
		{"P.S. I Love You", "P.S.I.Love.You.2007.1080p.BluRay.x264-GRP", true},
		{"P.S. I Love You", "PS.I.Love.You.2007.1080p.BluRay.x264-GRP", true},
		{"S.W.A.T.", "S.W.A.T.2017.S01E01.1080p.WEB.h264-GRP", true},
		{"S.W.A.T.", "SWAT.2017.S01E01.1080p.WEB.h264-GRP", true},
		{"M*A*S*H", "M.A.S.H.S01E01.1080p.WEB.h264-GRP", true},
		{"M*A*S*H", "MASH.S01E01.1080p.WEB.h264-GRP", true},
		{"Marvel's Agents of S.H.I.E.L.D.", "Marvels.Agents.of.SHIELD.S01E01.1080p.WEB-DL", true},
		{"Schitt's Creek", "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", true},
		{"Agents of S.H.I.E.L.D.", "The.Shield.S01E01.1080p.WEB.h264-GRP", false},
		{"S.W.A.T.", "Swatch.Story.2017.1080p.WEB.h264-GRP", false},
		{"Schitt's Creek", "Creek.S04E02.1080p.WEB.h264-GRP", false},
		// a one-letter word next to a codec letter still counts
		{"Plan B", "Plan.B.H.264.WEB", true},
		{"Malcolm X", "Malcolm.X.H.264", true},
		{"Malcolm X", "Malcolm.in.the.Middle.H.264", false},
		{"D.Gray-man", "D.Gray-man.S01E01.1080p.WEB.h264-GRP", true},
		{"Jackass 2.5", "Jackass.2.5.2007.DVDRip.XviD-GRP", true},
		// a one-letter title is never found inside letters written apart
		{"W.", "S.W.A.T.2017.S08E01.1080p.WEB.h264-GRP", false},
		{"M", "House.M.D.S01E01.1080p.WEB.h264-GRP", false},
		{"Q", "Some.Show.S01E01.1080p.WEB.x264-Q.C", false},
		{"V", "V.2009.S01E01.1080p.WEB.h264-GRP", true},
		{"M", "M.1931.1080p.BluRay.x264-GRP", true},
	} {
		if got := namesAll(c.release, nameWords(c.title)); got != c.want {
			t.Errorf("namesAll(%q, %q) = %v, want %v", c.release, c.title, got, c.want)
		}
	}
}

// TestTextQuery_DropsAccents checks that accented Latin letters go plain, as
// the older apps send them: sites often find "Pokemon" but not "Pokémon".
// Other scripts stay as they are.
func TestTextQuery_DropsAccents(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Pokémon", "Pokemon"},
		{"Amélie", "Amelie"},
		{"ÉLITE", "ELITE"},
		{"Señor Ávila", "Senor Avila"},
		{"Shingeki no Kyojin", "Shingeki no Kyojin"},
		{"進撃の巨人", "進撃の巨人"},
	} {
		if got := textQuery(Query{Title: c.in}); got != c.want {
			t.Errorf("textQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
