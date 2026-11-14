package sources

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Library title -> the search text the app sends -> a release named the way
// indexers write it -> the title match must accept it (as a feed item, and
// as the importer's show-hinted read, which share MatchTitle).
func TestApostropheTitlesSearchThenAccept(t *testing.T) {
	lib := []mediafiles.TitleRef{
		{ID: 1, Kind: "show", Name: "Schitt's Creek"},
		{ID: 16, Kind: "show", Name: "Parks’ Rec"},
		{ID: 2, Kind: "show", Name: "Grey's Anatomy"},
		{ID: 3, Kind: "show", Name: "It's Always Sunny in Philadelphia"},
		{ID: 4, Kind: "show", Name: "Bob's Burgers"},
		{ID: 5, Kind: "show", Name: "Marvel's Agents of S.H.I.E.L.D."},
		{ID: 6, Kind: "show", Name: "Law & Order: Special Victims Unit", Aliases: []string{"Law and Order SVU"}},
		{ID: 7, Kind: "show", Name: "Mr. Robot"},
		{ID: 8, Kind: "show", Name: "Tom Clancy's Jack Ryan"},
		{ID: 9, Kind: "movie", Name: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001},
		{ID: 10, Kind: "movie", Name: "Ocean's Eleven", Year: 2001},
		{ID: 11, Kind: "movie", Name: "Don't Look Up", Year: 2021},
		{ID: 12, Kind: "artist", Name: "Guns N' Roses"},
		{ID: 14, Kind: "show", Name: "Hell's Kitchen"},
		{ID: 15, Kind: "show", Name: "Hells Kitchen"},
	}
	rows := []struct {
		name    string
		title   string // library name searched for
		year    int
		release string
		want    int64 // 0: the match must report no single title
		skip    string
	}{
		{"schitts", "Schitt's Creek", 0, "Schitts.Creek.S04E02.1080p.WEB.h264-GRP", 1, ""},
		{"schitt s", "Schitt's Creek", 0, "Schitt.s.Creek.S04E02.1080p.WEB.h264-GRP", 1, ""},
		{"schitt apostrophe", "Schitt's Creek", 0, "Schitt's.Creek.S04E02.1080p.WEB.h264-GRP", 1, ""},
		{"greys", "Grey's Anatomy", 0, "Greys.Anatomy.S20E01.1080p.WEB.h264-GRP", 2, ""},
		{"its always sunny", "It's Always Sunny in Philadelphia", 0, "Its.Always.Sunny.in.Philadelphia.S16E01.1080p.WEB.h264-GRP", 3, ""},
		{"bobs", "Bob's Burgers", 0, "Bobs.Burgers.S14E01.1080p.WEB.h264-GRP", 4, ""},
		{"marvels dots", "Marvel's Agents of S.H.I.E.L.D.", 0, "Marvels.Agents.of.S.H.I.E.L.D.S07E01.1080p.WEB.h264-GRP", 5, ""},
		{"marvels shield", "Marvel's Agents of S.H.I.E.L.D.", 0, "Marvels.Agents.of.SHIELD.S07E01.1080p.WEB.h264-GRP", 5,
			"known gap: letters joined by dots (SHIELD) do not match S.H.I.E.L.D. (S H I E L D)"},
		{"law and order", "Law & Order: Special Victims Unit", 0, "Law.and.Order.Special.Victims.Unit.S25E01.1080p.WEB.h264-GRP", 6, ""},
		{"law and order svu alias", "Law & Order: Special Victims Unit", 0, "Law.And.Order.SVU.S25E01.1080p.WEB.h264-GRP", 6, ""},
		{"mr robot", "Mr. Robot", 0, "Mr.Robot.S04E01.1080p.WEB.h264-GRP", 7, ""},
		{"jack ryan", "Tom Clancy's Jack Ryan", 0, "Tom.Clancys.Jack.Ryan.S04E01.1080p.WEB.h264-GRP", 8, ""},
		{"amelie", "Le Fabuleux Destin d'Amélie Poulain", 2001, "Le.Fabuleux.Destin.d.Amelie.Poulain.2001.1080p.BluRay.x264-GRP", 9, ""},
		{"amelie joined", "Le Fabuleux Destin d'Amélie Poulain", 2001, "Le.Fabuleux.Destin.dAmelie.Poulain.2001.1080p.BluRay.x264-GRP", 9, ""},
		{"oceans", "Ocean's Eleven", 2001, "Oceans.Eleven.2001.1080p.BluRay.x264-GRP", 10, ""},
		{"dont look up", "Don't Look Up", 2021, "Dont.Look.Up.2021.1080p.WEB.h264-GRP", 11, ""},
		{"guns n roses", "Guns N' Roses", 0, "Guns N Roses - Appetite for Destruction (1987) [FLAC]", 12, ""},
		{"curly library name", "Parks’ Rec", 0, "Parks.Rec.S01E01.1080p.WEB.h264-GRP", 16, ""},
		{"curly library name second", "Parks’ Rec", 0, "Parks.Rec.S01E01.1080p.WEB.h264-GRP", 16, ""},
		// Titles that differ only by the apostrophe: a release that keeps the apostrophe
		// names only the first; one without it fits both, and with no year to tell them
		// apart the match is unsure (no single title).
		{"hells with apostrophe", "Hell's Kitchen", 0, "Hell's.Kitchen.S20E01.1080p.WEB.h264-GRP", 14, ""},
		{"hells without", "Hells Kitchen", 0, "Hells.Kitchen.S20E01.1080p.WEB.h264-GRP", 0, ""},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			if r.skip != "" {
				t.Skip(r.skip)
			}
			if q := textQuery(Query{Title: r.title, Year: r.year}); q == "" {
				t.Fatal("empty search text")
			}
			m := matchFeedItem(Item{Title: r.release}, lib, true, true)
			if r.want == 0 {
				if m.Title != nil {
					t.Fatalf("want no single title, got %d (%s)", m.Title.ID, m.Detail)
				}
				return
			}
			if m.Title == nil || m.Title.ID != r.want {
				t.Fatalf("%s: got %+v", r.release, m)
			}
		})
	}
}
