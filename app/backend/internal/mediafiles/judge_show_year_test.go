package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// seasons builds a show's episodes: 13 in each season named, with running
// numbers counted through the seasons in order.
func seasons(ns ...int) []EpisodeRef {
	var out []EpisodeRef
	for _, s := range ns {
		for e := 1; e <= 13; e++ {
			out = append(out, EpisodeRef{ItemID: int64(s*100 + e), Season: s, Episode: e, Absolute: len(out) + 1})
		}
	}
	return out
}

// chooseShow is how a search or feed ties a release to a show: the name
// and its year pick the show, the numbers pick its episodes. 0: the
// release is not taken.
func chooseShow(rel string, lib []TitleRef, eps map[int64][]EpisodeRef) (int64, string) {
	p := releasename.Parse(rel, releasename.Hints{Show: true})
	m := MatchRelease(p, lib)
	if m.Title == nil {
		return 0, m.Detail
	}
	_, why := MatchEpisodes(p, "aired", eps[m.Title.ID])
	if why != "" {
		return 0, why
	}
	return m.Title.ID, m.Detail
}

func TestShowYearInName(t *testing.T) {
	show := func(id int64, name string, year int, aliases ...string) TitleRef {
		return TitleRef{ID: id, Name: name, Year: year, Kind: "show", Aliases: aliases}
	}
	flash90, flash14 := show(1, "The Flash", 1990), show(2, "The Flash", 2014)
	ghostsUK, ghostsUS := show(3, "Ghosts", 2019), show(4, "Ghosts", 2021)
	who63, who05 := show(5, "Doctor Who", 1963), show(6, "Doctor Who", 2005)
	who24 := show(7, "Doctor Who", 2024, "Doctor Who (2023)")
	greys := show(8, "Grey's Anatomy", 2005)
	bsg78, bsg04 := show(9, "Battlestar Galactica", 1978), show(10, "Battlestar Galactica", 2004)
	marsUS := show(11, "Life on Mars", 2008)
	y1923, paris, space := show(12, "1923", 2022), show(13, "Paris Police 1900", 2021), show(14, "Space: 1999", 1975)
	girls := show(15, "2 Broke Girls", 2011)
	zone19 := show(16, "The Twilight Zone", 2019)
	hunter11 := show(17, "Hunter x Hunter", 2011)
	daily := show(18, "The Daily Show", 1996)
	noYear := show(19, "Copper Valley", 0)
	noDays := show(20, "Glass Orchard", 2010)
	futurama := show(21, "Futurama", 1999)
	hazbin, amber := show(22, "Hazbin Hotel", 2024), show(23, "Amber Ridge", 2021)
	eps := map[int64][]EpisodeRef{
		1: seasons(1), 2: seasons(1, 9),
		3: seasons(1, 2, 3, 4), 4: seasons(1),
		5: seasons(1, 12), 6: seasons(1, 12, 13),
		7:  seasons(1),
		8:  seasons(1, 16),
		9:  seasons(1),
		10: seasons(1),
		11: seasons(1),
		12: seasons(1), 13: seasons(1), 14: seasons(1),
		15: seasons(1), 16: seasons(1),
		17: seasons(1),
		18: {{ItemID: 1801, Season: 28, Episode: 100, AirDay: "2023-05-14"}},
		19: seasons(1), 20: seasons(1, 3),
		21: seasons(1, 8),
		24: seasons(1),
		22: seasons(0, 1), 23: seasons(1),
	}
	for _, c := range []struct {
		why  string
		lib  []TitleRef
		rel  string
		want int64 // the show it belongs to; 0: not taken
	}{
		// What the older app does with each name is in its manual,
		// backend/search-terms.md: section 5 (the year must equal the show's
		// year; the name with the year attached is tried first; with no
		// year the name alone decides and a tie is not guessed) and
		// sections 2 and 4 (a show's other names).
		{"the year is the show's, the name is another name of the show", []TitleRef{show(24, "Wings of the Wind", 2021, "Kaze no Tsubasa")}, "Kaze.no.Tsubasa.2021.S01E01.1080p.WEB.h264-GRP", 24},
		{"another name of the show with a year that is not the show's", []TitleRef{show(24, "Wings of the Wind", 2021, "Kaze no Tsubasa")}, "Kaze.no.Tsubasa.2019.S01E01.1080p.WEB.h264-GRP", 0},
		{"another name of the show with no year", []TitleRef{show(24, "Wings of the Wind", 2021, "Kaze no Tsubasa")}, "Kaze.no.Tsubasa.S01E01.1080p.WEB.h264-GRP", 24},
		{"older same-name show, only the newer one in the library", []TitleRef{flash14}, "The.Flash.1990.S01E01.1080p.BluRay.x264-GRP", 0},
		{"newer same-name show, only the older one in the library", []TitleRef{flash90}, "The.Flash.2014.S01E01.1080p.WEB.h264-GRP", 0},
		{"both in the library: the year picks", []TitleRef{flash90, flash14}, "The.Flash.1990.S01E01.1080p.BluRay.x264-GRP", 1},
		{"both in the library: the year picks", []TitleRef{flash90, flash14}, "The.Flash.2014.S01E01.1080p.WEB.h264-GRP", 2},
		{"the show's own year", []TitleRef{flash14}, "The.Flash.2014.S09E01.1080p.WEB.h264-GRP", 2},
		{"a season's year is not the show's", []TitleRef{flash14}, "The.Flash.2023.S09E01.1080p.WEB.h264-GRP", 0},
		{"a year after the episode number is not the show's", []TitleRef{flash14}, "The.Flash.S01E01.2023.REMASTERED.1080p.WEB.h264-GRP", 2},
		{"no year in the name", []TitleRef{flash14}, "The.Flash.S01E01.1080p.WEB.h264-GRP", 2},
		{"no year and two shows of the name: no guess", []TitleRef{flash90, flash14}, "The.Flash.S01E01.1080p.WEB.h264-GRP", 0},
		{"same name two years apart, only the newer one in the library", []TitleRef{ghostsUS}, "Ghosts.2019.S01E01.1080p.WEB.H264-GRP", 0},
		{"same name two years apart, only the older one in the library", []TitleRef{ghostsUK}, "Ghosts.2021.S01E01.1080p.WEB.H264-GRP", 0},
		{"same name two years apart, both in the library", []TitleRef{ghostsUK, ghostsUS}, "Ghosts.2021.S01E01.1080p.WEB.H264-GRP", 4},
		{"same name two years apart, only the newer one in the library", []TitleRef{marsUS}, "Life.on.Mars.2006.S01E01.720p.HDTV.x264-GRP", 0},
		{"a later show of the name while this one still runs", []TitleRef{who05}, "Doctor.Who.2023.S01E01.1080p.WEB.h264-GRP", 0},
		{"the show's own year on a late season", []TitleRef{who05}, "Doctor.Who.2005.S13E01.1080p.WEB.h264-GRP", 6},
		{"the show's own year on a late season pack", []TitleRef{who05}, "Doctor.Who.2005.S13.1080p.BluRay.x264-GRP", 6},
		{"a season's year, two shows of the name in the library", []TitleRef{who63, who05}, "Doctor.Who.2021.S13E01.1080p.WEB.h264-GRP", 0},
		{"a season's year of the older show, two in the library", []TitleRef{who63, who05}, "Doctor.Who.1974.S12E01.DVDRip.x264-GRP", 0},
		{"the year is in another name of the show", []TitleRef{who24}, "Doctor.Who.2023.S01E01.1080p.WEB.h264-GRP", 7},
		{"a season's year, show began long before", []TitleRef{greys}, "Greys.Anatomy.2019.S16E01.720p.HDTV.x264-GRP", 0},
		{"a season's year on a later episode", []TitleRef{greys}, "Greys.Anatomy.2019.S16E13.720p.HDTV.x264-GRP", 0},
		{"a season pack with the season's year", []TitleRef{greys}, "Greys.Anatomy.2019.S16.1080p.WEB.h264-GRP", 0},
		{"some other year", []TitleRef{greys}, "Greys.Anatomy.2012.S16E01.720p.HDTV.x264-GRP", 0},
		{"a revival season's year is not the show's", []TitleRef{futurama}, "Futurama.2023.S08E01.1080p.WEB.h264-GRP", 0},
		{"a year one off from the show's is not the show's", []TitleRef{bsg04}, "Battlestar.Galactica.2003.S01E01.720p.BluRay.x264-GRP", 0},
		{"one off, with an older show of the name in the library", []TitleRef{bsg78, bsg04}, "Battlestar.Galactica.2003.S01E01.720p.BluRay.x264-GRP", 0},
		{"the older one of the two", []TitleRef{bsg78, bsg04}, "Battlestar.Galactica.1978.S01E01.720p.BluRay.x264-GRP", 9},
		{"the name is a year", []TitleRef{y1923}, "1923.S01E01.1080p.WEB.h264-GRP", 12},
		{"the name ends in a year", []TitleRef{paris}, "Paris.Police.1900.S01E01.FRENCH.1080p.WEB.H264-GRP", 13},
		{"the name ends in a year", []TitleRef{space}, "Space.1999.S01E01.1080p.BluRay.x264-GRP", 14},
		{"the name ends in a year, and the show's year follows", []TitleRef{space}, "Space.1999.1975.S01E01.1080p.BluRay.x264-GRP", 14},
		{"a number in the name and the show's year", []TitleRef{girls}, "2.Broke.Girls.2011.S01E01.720p.HDTV.x264-GRP", 15},
		{"an older show of the name, far back", []TitleRef{zone19}, "The.Twilight.Zone.1959.S01E01.1080p.BluRay.x264-GRP", 0},
		{"an older show of the name", []TitleRef{zone19}, "The.Twilight.Zone.2002.S01E01.DVDRip.x264-GRP", 0},
		{"anime with the show's year", []TitleRef{hunter11}, "[SubsPlease] Hunter x Hunter (2011) - 01 (1080p) [ABCD1234].mkv", 17},
		{"anime with the older show's year", []TitleRef{hunter11}, "[SubsPlease] Hunter x Hunter (1999) - 01 (1080p) [ABCD1234].mkv", 0},
		{"a daily show: the date is not the show's year", []TitleRef{daily}, "The.Daily.Show.2023.05.14.1080p.WEB.h264-GRP", 18},
		{"the library has no year for the show: any year is taken, there is nothing to compare", []TitleRef{noYear}, "Copper.Valley.2019.S01E01.1080p.WEB.h264-GRP", 19},
		{"a later year is not the show's", []TitleRef{noDays}, "Glass.Orchard.2012.S03E01.1080p.WEB.h264-GRP", 0},
		{"the other show's year on season 2", []TitleRef{ghostsUK}, "Ghosts.2021.S02E01.1080p.WEB.H264-GRP", 0},
		{"the other show's year on season 4", []TitleRef{ghostsUK}, "Ghosts.2021.S04E01.1080p.WEB.H264-GRP", 0},
		{"the other show's year on a season pack", []TitleRef{ghostsUK}, "Ghosts.2021.S02.1080p.WEB.H264-GRP", 0},
		{"the other show's year on season 3", []TitleRef{ghostsUK}, "Ghosts.2021.S03E01.1080p.WEB.H264-GRP", 0},
		{"a year before the show's listed year, on a special", []TitleRef{hazbin}, "Hazbin.Hotel.2019.S00E01.1080p.WEB.h264-GRP", 0},
		{"a year before the show's listed year", []TitleRef{amber}, "Amber.Ridge.2019.S01E01.1080p.WEB.h264-GRP", 0},
		{"two shows of the name, each a year off: no guess", []TitleRef{ghostsUK, ghostsUS}, "Ghosts.2020.S01E01.1080p.WEB.H264-GRP", 0},
		{"two shows of the name, each a year off, other order: no guess", []TitleRef{ghostsUS, ghostsUK}, "Ghosts.2020.S01E01.1080p.WEB.H264-GRP", 0},
		{"an earlier year is not the show's", []TitleRef{noDays}, "Glass.Orchard.2001.S01E01.1080p.WEB.h264-GRP", 0},
	} {
		got, detail := chooseShow(c.rel, c.lib, eps)
		if got != c.want {
			t.Errorf("FAIL %-58s want show %d, got %d (%s) [%s]", c.rel, c.want, got, detail, c.why)
			continue
		}
		t.Logf("ok   %-58s show %d (%s) [%s]", c.rel, got, detail, c.why)
	}
}

// A file on disk or a finished download keeps the loose match: its year is
// never a reason to leave it out, and a name ending in a year still matches.
func TestShowYearOnImport(t *testing.T) {
	lib := []TitleRef{{ID: 1, Name: "The Flash", Year: 2014, Kind: "show"}, {ID: 2, Name: "Paris Police 1900", Year: 2021, Kind: "show"}}
	for name, want := range map[string]int64{
		"The Flash (1990) - S01E01 - Pilot.mkv":              1,
		"The.Flash.2023.S09E01.1080p.WEB.h264-GRP.mkv":       1,
		"Paris Police 1900 - S01E01 - Episode 1.mkv":         2,
		"Paris.Police.1900.S01E01.FRENCH.1080p.WEB.H264-GRP": 2,
	} {
		m := MatchTitle(releasename.Parse(name, releasename.Hints{Show: true}), lib)
		if m.Title == nil || m.Title.ID != want {
			t.Errorf("FAIL %s: want show %d, got %+v (%s)", name, want, m.Title, m.Detail)
		}
	}
}
