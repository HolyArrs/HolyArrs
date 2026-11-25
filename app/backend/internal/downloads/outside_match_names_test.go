package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Names seen in a client after moving from old apps match the Library's
// titles, accents and punctuation aside; software and games never do.
func TestOutsideNamesMatchLibraryTitles(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, title, kind string }{
		{"90 Day Fiance The Other Way S07E03 Naya Safar 1080p HMax WEB-DL DDP2 0 H 264-STC", "90 Day Fiancé: The Other Way", "show"},
		{"90.Day.Fiance.The.Other.Way.S07E18.1080p.WEB.h264-FREQUENCY", "90 Day Fiancé: The Other Way", "show"},
		{"Tulsa.King.S04E04.GERMAN.DL.1080p.WEB.h264.PROPER-SAUERKRAUT", "Tulsa King", "show"},
		{"MobLand S02E10 1080p 10bit WEBRip 6CH x265 HEVC-PSA", "MobLand", "show"},
		{"Ordinary.Angels.2024.Hybrid.2160p.WEB-DL.DV.HDR.DDP5.1.Atmos.H265-AOC", "Ordinary Angels", "movie"},
		{"Pokemon.S01E01.1080p.WEB.h264-GRP", "Pokémon", "show"},
		{"Bobs.Burgers.S01E01.1080p.WEB.h264-GRP", "Bob's Burgers", "show"},
	} {
		g := guessOf(c.name, "")
		if g.kind != c.kind {
			t.Errorf("%q kind %q", c.name, g.kind)
		}
		if s := likeness(g.title, c.title); s != 100 {
			t.Errorf("%q vs %q: %d", g.title, c.title, s)
		}
	}
	for _, n := range []string{"Metal.Slug.Collection-RELOADED", "CCleaner.Professional.Plus.6.35.Multilingual"} {
		if g := guessOf(n, ""); g.kind != "" {
			t.Errorf("%q read as %q", n, g.kind)
		}
	}
}

// Only items in HolyArrs's own categories are matched on a torrent client;
// other apps' TV or movie categories and look-alike names are left alone.
func TestOutsideTorrentCategories(t *testing.T) {
	t.Parallel()
	qb := domain.Downloader{Protocol: "torrent", Category: "holyarrs"}
	for _, c := range []struct {
		cat  string
		want bool
	}{
		{"holyarrs", false}, {"HolyArrs", false},
		{"tv-sonarr", true}, {"radarr", true}, {"prowlarr", true}, {"Movies", true}, {"tv", true},
		{"showcase", true}, {"hdtv-games", true}, {"films-docs", true},
		{"games", true}, {"software", true}, {"", true},
	} {
		if got := otherApp(qb, c.cat); got != c.want {
			t.Errorf("%q: %v", c.cat, got)
		}
	}
	if otherApp(domain.Downloader{Protocol: "torrent"}, "games") {
		t.Error("a client with no category has every item as its own")
	}
}
