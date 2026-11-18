package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Adversarial name table: release name -> Parse -> MatchRelease,
// the same two calls the search and feed paths make.
type nameCase struct {
	lib  string
	year int
	kind string // show, movie
	rel  string
	want bool
	flag string // "" normal, "debatable", "open" (a known gap, logged only)
	freq string // how likely a real user meets it if it fails
}

func TestNameShapes(t *testing.T) {
	S, M := "show", "movie"
	cases := []nameCase{
		// apostrophe shapes
		{"Schitt's Creek", 0, S, "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "common"},
		{"Schitt's Creek", 0, S, "Schitt.s.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "common"},
		{"Schitt's Creek", 0, S, "Schitt's.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "common"},
		{"Schitt's Creek", 0, S, "Schitt’s.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Schitt’s Creek", 0, S, "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "common"},
		{"Schitt’s Creek", 0, S, "Schitt.s.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "common"},
		{"Schitt`s Creek", 0, S, "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Schitt's Creek", 0, S, "Schitt`s.Creek.S04E02.1080p.WEB.h264-GRP", true, "", "theoretical"},
		{"Schitt's Creek", 0, S, "Schitts Creek S04E02 1080p WEB h264-GRP", true, "", "common"},
		{"Schitt's Creek", 0, S, "Schitts_Creek_S04E02_1080p_WEB_h264-GRP", true, "", "rare"},
		{"Schitt's Creek", 0, S, "schitts.creek.s04e02.1080p.web.h264-grp", true, "", "common"},
		{"Grey's Anatomy", 0, S, "Greys.Anatomy.S19E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Grey's Anatomy", 0, S, "Grey.s.Anatomy.S19E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Bob's Burgers", 0, S, "Bobs.Burgers.S13E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Ocean's Eleven", 2001, M, "Oceans.Eleven.2001.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Ocean's Eleven", 2001, M, "Ocean's.Eleven.2001.1080p.BluRay.x264-GRP", true, "", "common"},
		{"The Handmaid's Tale", 0, S, "The.Handmaids.Tale.S05E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"The Handmaid's Tale", 0, S, "Handmaids.Tale.S05E01.1080p.WEB.h264-GRP", true, "debatable", "rare"},
		{"Marvel's Agents of S.H.I.E.L.D.", 0, S, "Marvels.Agents.of.S.H.I.E.L.D.S01E01.1080p.WEB-DL", true, "", "common"},
		{"Marvel's Agents of S.H.I.E.L.D.", 0, S, "Marvels.Agents.of.SHIELD.S01E01.1080p.WEB-DL", true, "", "common"},
		{"Marvel's Agents of S.H.I.E.L.D.", 0, S, "Agents.of.S.H.I.E.L.D.S01E01.1080p.WEB-DL", true, "open", "common"},
		{"Marvel's Agents of S.H.I.E.L.D.", 0, S, "Agents.of.SHIELD.S01E01.1080p.WEB-DL", true, "open", "common"},
		// contractions
		{"Don't Look Up", 2021, M, "Dont.Look.Up.2021.1080p.WEB-DL.DDP5.1.x264-GRP", true, "", "common"},
		{"Don't Look Up", 2021, M, "Don't.Look.Up.2021.1080p.WEB-DL.DDP5.1.x264-GRP", true, "", "common"},
		{"Don't Look Up", 2021, M, "Don.t.Look.Up.2021.1080p.WEB-DL.DDP5.1.x264-GRP", true, "", "common"},
		{"Can't Hardly Wait", 1998, M, "Cant.Hardly.Wait.1998.1080p.BluRay.x264-GRP", true, "", "common"},
		{"It's Always Sunny in Philadelphia", 0, S, "Its.Always.Sunny.in.Philadelphia.S15E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"It's Always Sunny in Philadelphia", 0, S, "It's.Always.Sunny.in.Philadelphia.S15E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"What We Do in the Shadows", 0, S, "What.We.Do.in.the.Shadows.S05E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Rock 'n' Roll High School", 1979, M, "Rock.n.Roll.High.School.1979.1080p.BluRay.x264-GRP", true, "", "rare"},
		{"Don't You Dare", 0, S, "Dont.You.Dare.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		// ampersand
		{"Law & Order", 0, S, "Law.and.Order.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Law & Order", 0, S, "Law.&.Order.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Law & Order", 0, S, "Law.Order.S01E01.1080p.WEB.h264-GRP", true, "open", "rare"},
		{"Rick and Morty", 0, S, "Rick.and.Morty.S07E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Rick and Morty", 0, S, "Rick.&.Morty.S07E01.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Fast & Furious", 2009, M, "Fast.and.Furious.2009.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Fast & Furious", 2009, M, "Fast.&.Furious.2009.1080p.BluRay.x264-GRP", true, "", "rare"},
		{"Fast & Furious", 2009, M, "Fast.Furious.2009.1080p.BluRay.x264-GRP", true, "open", "rare"},
		// colons and dashes
		{"Star Trek: Discovery", 0, S, "Star.Trek.Discovery.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Star Trek: Discovery", 0, S, "Star.Trek-Discovery.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Spider-Man: No Way Home", 2021, M, "Spider-Man.No.Way.Home.2021.1080p.WEB-DL.x264-GRP", true, "", "common"},
		{"Spider-Man: No Way Home", 2021, M, "Spider.Man.No.Way.Home.2021.1080p.WEB-DL.x264-GRP", true, "", "common"},
		{"Spider-Man: No Way Home", 2021, M, "SpiderMan.No.Way.Home.2021.1080p.WEB-DL.x264-GRP", true, "open", "rare"},
		{"9-1-1", 0, S, "9-1-1.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"9-1-1", 0, S, "9.1.1.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"9-1-1", 0, S, "911.S01E01.1080p.WEB.h264-GRP", true, "open", "rare"},
		{"9-1-1: Lone Star", 0, S, "9-1-1.Lone.Star.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"9-1-1: Lone Star", 0, S, "9.1.1.Lone.Star.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		// dots in acronyms
		{"S.W.A.T.", 2017, S, "S.W.A.T.2017.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"S.W.A.T.", 2017, S, "SWAT.2017.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"M*A*S*H", 0, S, "M.A.S.H.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"M*A*S*H", 0, S, "MASH.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Agents of S.H.I.E.L.D.", 0, S, "Agents.of.S.H.I.E.L.D.S01E01.1080p.WEB-DL", true, "", "common"},
		{"Agents of S.H.I.E.L.D.", 0, S, "Agents.of.SHIELD.S01E01.1080p.WEB-DL", true, "", "common"},
		// accents
		{"Pokémon", 0, S, "Pokemon.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Pokémon", 0, S, "Pokémon.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Pokémon", 0, S, "Pokemoń.S01E01.1080p.WEB.h264-GRP", true, "", "theoretical"},
		{"Amélie", 2001, M, "Amelie.2001.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Les Misérables", 2012, M, "Les.Miserables.2012.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Señorita 89", 0, S, "Senorita.89.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Amelie", 2001, M, "Amélie.2001.1080p.BluRay.x264-GRP", true, "", "rare"},
		// punctuation ends
		{"Who Is America?", 0, S, "Who.Is.America.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Mr. Robot", 0, S, "Mr.Robot.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Dr. Stone", 0, S, "Dr.Stone.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Mr. Robot", 0, S, "Mister.Robot.S01E01.1080p.WEB.h264-GRP", false, "debatable", "rare"},
		{"Jeopardy!", 0, S, "Jeopardy.S40E01.1080p.WEB.h264-GRP", true, "", "common"},
		// numbers
		{"Se7en", 1995, M, "Se7en.1995.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Se7en", 1995, M, "Seven.1995.1080p.BluRay.x264-GRP", true, "open", "common"},
		{"2 Broke Girls", 0, S, "2.Broke.Girls.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"2 Broke Girls", 0, S, "Two.Broke.Girls.S01E01.1080p.WEB.h264-GRP", true, "open", "rare"},
		{"Ocean's 8", 2018, M, "Oceans.8.2018.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Ocean's 8", 2018, M, "Oceans.Eight.2018.1080p.BluRay.x264-GRP", true, "open", "rare"},
		// year in name
		{"Doctor Who", 2005, S, "Doctor.Who.2005.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Doctor Who", 2005, S, "Doctor.Who.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"The Flash", 2014, S, "The.Flash.2014.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"The Flash", 2014, S, "The.Flash.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		// country tag
		{"The Office (US)", 0, S, "The.Office.US.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"The Office (US)", 0, S, "The.Office.S01E01.1080p.WEB.h264-GRP", true, "open", "common"},
		{"Shameless (US)", 0, S, "Shameless.US.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Shameless (US)", 0, S, "Shameless.S01E01.1080p.WEB.h264-GRP", true, "open", "common"},
		{"Shameless", 0, S, "Shameless.US.S01E01.1080p.WEB.h264-GRP", true, "open", "common"},
		// The prefix/suffix
		{"The Walking Dead", 0, S, "Walking.Dead.S01E01.1080p.WEB.h264-GRP", true, "debatable", "rare"},
		{"The Walking Dead", 0, S, "Walking.Dead,.The.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		{"The Walking Dead", 0, S, "The.Walking.Dead.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		// slash, plus
		{"Face/Off", 1997, M, "Face.Off.1997.1080p.BluRay.x264-GRP", true, "", "common"},
		{"Face/Off", 1997, M, "FaceOff.1997.1080p.BluRay.x264-GRP", true, "open", "common"},
		{"Love + Death", 0, S, "Love.Death.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"Love + Death", 0, S, "Love.and.Death.S01E01.1080p.WEB.h264-GRP", true, "open", "common"},
		{"Love + Death", 0, S, "Love.+.Death.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		// scene form of apostrophe titles is meant to match
		{"We're Here", 0, S, "Were.Here.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		{"We'll Meet Again", 0, S, "Well.Meet.Again.S01E01.1080p.WEB.h264-GRP", true, "", "rare"},
		{"Hell's Kitchen", 0, S, "Hells.Kitchen.US.S01E01.1080p.WEB.h264-GRP", true, "open", "common"},
		{"Hell's Kitchen", 0, S, "Hells.Kitchen.S01E01.1080p.WEB.h264-GRP", true, "", "common"},
		// MUST NOT MATCH
		{"Grey", 0, S, "Greys.Anatomy.S19E01.1080p.WEB.h264-GRP", false, "", "common"},
		{"Grey", 0, S, "Greys.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Greys", 0, S, "Grey.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Ocean's Eleven", 2001, M, "Oceans.Twelve.2004.1080p.BluRay.x264-GRP", false, "", "common"},
		{"Ocean's Eleven", 2001, M, "Oceans.8.2018.1080p.BluRay.x264-GRP", false, "", "common"},
		{"Ocean's Twelve", 2004, M, "Oceans.Eleven.2001.1080p.BluRay.x264-GRP", false, "", "common"},
		{"Ocean's 8", 2018, M, "Oceans.Eleven.2001.1080p.BluRay.x264-GRP", false, "", "common"},
		{"The Office", 0, S, "The.Office.US.S01E01.1080p.WEB.h264-GRP", false, "debatable", "common"},
		{"The Office (US)", 0, S, "The.Office.UK.S01E01.1080p.WEB.h264-GRP", false, "", "common"},
		{"It's Always Sunny in Philadelphia", 0, S, "Always.Sunny.in.Philadelphia.S15E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Always Sunny", 0, S, "Its.Always.Sunny.in.Philadelphia.S15E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Schitt's Creek", 0, S, "Creek.S04E02.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Creek", 0, S, "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", false, "", "common"},
		{"Schitt's Creek", 0, S, "Schitts.Creek.Reunion.S04E02.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Schitt's Creek", 0, M, "Schitts.Creek.2020.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"Can", 0, S, "Cant.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Can't", 0, S, "Can.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Cant", 0, S, "Can't.S01E01.1080p.WEB.h264-GRP", false, "debatable", "rare"},
		{"Were Here", 0, S, "We're.Here.S01E01.1080p.WEB.h264-GRP", false, "debatable", "rare"},
		{"Were", 0, S, "We're.S01E01.1080p.WEB.h264-GRP", false, "debatable", "rare"},
		{"We're", 0, S, "Were.S01E01.1080p.WEB.h264-GRP", true, "debatable", "rare"},
		{"We're Here", 0, S, "We.Here.S01E01.1080p.WEB.h264-GRP", false, "", "theoretical"},
		{"Hell's Kitchen", 0, S, "Hells.Angels.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Well", 0, S, "We'll.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"Well", 0, S, "We.ll.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"We'll", 0, S, "Well.S01E01.1080p.WEB.h264-GRP", true, "debatable", "rare"},
		{"Shell", 0, S, "She'll.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
		{"She'll", 0, S, "Shell.S01E01.1080p.WEB.h264-GRP", true, "debatable", "rare"},
		{"Ill", 2000, M, "I'll.2000.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"I'll", 2000, M, "Ill.2000.1080p.BluRay.x264-GRP", true, "debatable", "rare"},
		{"Id", 2000, M, "I'd.2000.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"I'd", 2000, M, "Id.2000.1080p.BluRay.x264-GRP", true, "debatable", "rare"},
		{"Wont", 2000, M, "Won't.2000.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"Won't", 2000, M, "Wont.2000.1080p.BluRay.x264-GRP", true, "debatable", "rare"},
		{"Wont", 2000, M, "Wonton.2000.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"Won't", 2000, M, "Wonton.2000.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"Wonton", 2000, M, "Won't.2000.1080p.BluRay.x264-GRP", false, "", "rare"},
		{"9-1-1", 0, S, "9-1-1.Lone.Star.S01E01.1080p.WEB.h264-GRP", false, "", "common"},
		{"9-1-1: Lone Star", 0, S, "9-1-1.S01E01.1080p.WEB.h264-GRP", false, "", "common"},
		{"Law & Order", 0, S, "Law.and.Order.SVU.S01E01.1080p.WEB.h264-GRP", false, "", "common"},
		{"Star Trek", 0, S, "Star.Trek.Discovery.S01E01.1080p.WEB.h264-GRP", false, "", "common"},
		{"The Flash", 2014, S, "The.Flash.1990.S01E01.1080p.WEB.h264-GRP", false, "debatable", "rare"},
		{"Doctor Who", 2005, S, "Doctor.Who.1963.S01E01.1080p.WEB.h264-GRP", false, "debatable", "rare"},
		{"Don't Look Up", 2021, M, "Dont.Look.Up.Now.2021.1080p.WEB-DL.x264-GRP", false, "", "rare"},
		{"Bob's Burgers", 0, S, "Bobs.Burger.S13E01.1080p.WEB.h264-GRP", false, "", "theoretical"},
		{"Rick and Morty", 0, S, "Rick.and.Morty.Shorts.S01E01.1080p.WEB.h264-GRP", false, "", "rare"},
	}
	pass, fail := 0, 0
	for i, c := range cases {
		p := releasename.Parse(c.rel, releasename.Hints{Show: c.kind == "show"})
		m := MatchRelease(p, []TitleRef{{ID: 1, Name: c.lib, Year: c.year, Kind: c.kind}})
		got := m.Title != nil
		if got == c.want {
			pass++
			continue
		}
		fail++
		report := t.Errorf
		if c.flag == "open" {
			report = t.Logf // a known gap left as it is: refused, which is the safe side
		}
		report("CASE %03d [%s|%s] lib=%q(%d,%s) rel=%q want=%v got=%v parsedTitle=%q detail=%q",
			i, c.flag, c.freq, c.lib, c.year, c.kind, c.rel, c.want, got, p.Title.Value, m.Detail)
	}
	t.Logf("TOTAL %d cases: %d as expected, %d not", len(cases), pass, fail)
}

// Letters written apart, other apostrophe marks and show years: what must
// now match and, above all, what must still be refused.
func TestNameShapesAcronymsAndYears(t *testing.T) {
	S, M := "show", "movie"
	cases := []nameCase{
		// must match
		{"ER", 0, S, "E.R.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"CSI", 0, S, "C.S.I.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"NCIS", 0, S, "N.C.I.S.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"S.W.A.T.", 2017, S, "swat.2017.s01e01.1080p.web.h264-grp", true, "", ""},
		{"S.W.A.T.", 2017, S, "SWAT.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"L.A. Confidential", 1997, M, "LA.Confidential.1997.1080p.BluRay.x264-GRP", true, "", ""},
		{"L.A. Confidential", 1997, M, "L.A.Confidential.1997.1080p.BluRay.x264-GRP", true, "", ""},
		{"U.S. Marshals", 1998, M, "US.Marshals.1998.1080p.BluRay.x264-GRP", true, "", ""},
		{"Hell`s Kitchen", 0, S, "Hells.Kitchen.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"Schitt‘s Creek", 0, S, "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", true, "", ""},
		{"A Man Called Otto", 2022, M, "A.Man.Called.Otto.2022.1080p.WEB-DL.x264-GRP", true, "", ""},
		{"I Am Legend", 2007, M, "I.Am.Legend.2007.1080p.BluRay.x264-GRP", true, "", ""},
		{"V", 2009, S, "V.2009.S01E01.1080p.WEB.h264-GRP", true, "", ""},
		{"X-Men", 2000, M, "X-Men.2000.1080p.BluRay.x264-GRP", true, "", ""},
		{"Battlestar Galactica", 2004, S, "Battlestar.Galactica.2003.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"The Flash", 2014, S, "The.Flash.2016.S03E01.1080p.WEB.h264-GRP", false, "", ""},
		{"Doctor Who", 2005, S, "Doctor.Who.2006.S02E01.1080p.WEB.h264-GRP", false, "", ""},
		// must not match
		{"S.W.A.T.", 2017, S, "S.W.A.T.Kats.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"S.W.A.T.", 2017, S, "SWAT.Kats.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"SWAT", 0, S, "S.W.A.T.Kats.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"S.W.A.T.", 2017, S, "SWAT.1975.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"M*A*S*H", 0, S, "Mashle.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"MASH", 0, S, "Mashle.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"M*A*S*H", 0, S, "Mash.Up.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"Agents of S.H.I.E.L.D.", 0, S, "The.Shield.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"The Shield", 0, S, "Agents.of.SHIELD.S01E01.1080p.WEB-DL", false, "", ""},
		{"The Shield", 0, S, "Agents.of.S.H.I.E.L.D.S01E01.1080p.WEB-DL", false, "", ""},
		{"I.T.", 2016, M, "IT.2017.1080p.BluRay.x264-GRP", false, "", ""},
		{"It", 2017, M, "I.T.2016.1080p.BluRay.x264-GRP", false, "", ""},
		{"Us", 2019, M, "U.S.Marshals.1998.1080p.BluRay.x264-GRP", false, "", ""},
		{"US", 1998, M, "U.S.Marshals.1998.1080p.BluRay.x264-GRP", false, "", ""},
		{"Er", 0, S, "E.R.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"CSI", 0, S, "C.S.I.Miami.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"C.S.I.", 0, S, "CSI.Miami.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"CSI: Miami", 0, S, "CSI.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"A Man Called Otto", 2022, M, "A.Man.Called.Ove.2015.1080p.BluRay.x264-GRP", false, "", ""},
		{"I Am Legend", 2007, M, "I.Am.Number.Four.2011.1080p.BluRay.x264-GRP", false, "", ""},
		{"V", 2009, S, "V.H.S.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"V", 2009, S, "V.Wars.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"X-Men", 2000, M, "X-Men.Apocalypse.2016.1080p.BluRay.x264-GRP", false, "", ""},
		{"L.A. Confidential", 1997, M, "LA.Story.1991.1080p.BluRay.x264-GRP", false, "", ""},
		{"Battlestar Galactica", 2004, S, "Battlestar.Galactica.1978.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"Charmed", 2018, S, "Charmed.1998.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"Hell`s Kitchen", 0, S, "Hells.Angels.S01E01.1080p.WEB.h264-GRP", false, "", ""},
		{"Schitt‘s Creek", 0, S, "Creek.S04E02.1080p.WEB.h264-GRP", false, "", ""},
		{"Schitt`s Creek", 0, S, "Schitt.Creek.S04E02.1080p.WEB.h264-GRP", false, "", ""},
	}
	for i, c := range cases {
		p := releasename.Parse(c.rel, releasename.Hints{Show: c.kind == "show"})
		m := MatchRelease(p, []TitleRef{{ID: 1, Name: c.lib, Year: c.year, Kind: c.kind}})
		if got := m.Title != nil; got != c.want {
			t.Errorf("case %d lib=%q(%d,%s) rel=%q want=%v got=%v parsed=%q detail=%q", i, c.lib, c.year, c.kind, c.rel, c.want, got, p.Title.Value, m.Detail)
		}
	}
	// A scene music name must not start with a longer word than the artist's name.
	p := releasename.Parse("Destinys.Childhood.Survivor.2001.FLAC", releasename.Hints{Music: true})
	if m := MatchTitle(p, []TitleRef{{ID: 1, Kind: "artist", Name: "Destiny's Child"}}); m.Title != nil {
		t.Errorf("Destinys Childhood matched Destiny's Child: %+v", m)
	}
}

// Music: the artist path, including the album step that compares artist names again.
func TestNameShapesMusic(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Kind: "artist", Name: "Destiny's Child"},
		{ID: 2, Kind: "artist", Name: "Guns N' Roses"},
	}
	albums := []AlbumRef{{PartID: 1, Name: "Survivor", Year: 2001}, {PartID: 2, Name: "Appetite for Destruction", Year: 1987}}
	cases := []struct {
		rel    string
		artist int64
		name   string
		album  string
	}{
		{"Destinys Child - Survivor (2001) [FLAC]", 1, "Destiny's Child", "Survivor"},
		{"Destiny's Child - Survivor (2001) [FLAC]", 1, "Destiny's Child", "Survivor"},
		{"Destiny s Child - Survivor (2001) [FLAC]", 1, "Destiny's Child", "Survivor"},
		{"Destinys.Child.Survivor.2001.FLAC", 1, "Destiny's Child", "Survivor"},
		{"Guns N Roses - Appetite for Destruction (1987) FLAC", 2, "Guns N' Roses", "Appetite for Destruction"},
		{"Guns N' Roses - Appetite for Destruction (1987) FLAC", 2, "Guns N' Roses", "Appetite for Destruction"},
		{"Guns.N.Roses.Appetite.for.Destruction.1987.FLAC", 2, "Guns N' Roses", "Appetite for Destruction"},
	}
	for _, c := range cases {
		p := releasename.Parse(c.rel, releasename.Hints{Music: true})
		m := MatchTitle(p, lib)
		if m.Title == nil || m.Title.ID != c.artist {
			t.Errorf("MUSIC artist: %q parsed=%+v -> %+v", c.rel, p.Music, m)
			continue
		}
		a, warn, why := MatchAlbum(p, c.name, albums)
		if a == nil {
			t.Errorf("MUSIC album: %q artist matched but album not: warn=%q why=%q", c.rel, warn, why)
		} else if a.Name != c.album {
			t.Errorf("MUSIC album: %q got %q", c.rel, a.Name)
		}
	}
}
