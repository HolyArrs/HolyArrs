package lists

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Synchronicity (Remastered 2003)":    "Synchronicity",
		`Dreams - Promo 7" Edit`:             "Dreams",
		"Song - Official -":                  "Song",
		"Tití Me Preguntó RMX - Remix":       "Tití Me Preguntó",
		"Thriller 25 (Super Deluxe Edition)": "Thriller 25",
		"Fearless (Taylor's Version)":        "Fearless (Taylor's Version)",
		"Credit Where It's Due":              "Credit Where It's Due",
		"(Remastered)":                       "(Remastered)",
	} {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

type listSearch []musictrend.Candidate

func (l listSearch) Search(context.Context, string, string, string) ([]musictrend.Candidate, error) {
	return append([]musictrend.Candidate{}, l...), nil
}

func TestHonestSearchPrefersTheOriginal(t *testing.T) {
	cs := listSearch{
		{MBID: "c", Name: "Abbey Road", Date: "2019-09-27", Secondary: []string{"Compilation"}},
		{MBID: "l", Name: "Abbey Road", Date: "1970-01-01", Secondary: []string{"Live"}},
		{MBID: "r", Name: "Abbey Road", Date: "2009-09-09"},
		{MBID: "o", Name: "Abbey Road", Date: "1969-09-26"},
		{MBID: "g", Name: "Greatest Hits", Date: "1981-10-26"},
	}
	got, _ := honestSearch{cs}.Search(context.Background(), "album", "Abbey Road", "The Beatles")
	if len(got) != 2 || got[0].MBID != "o" || got[1].MBID != "r" {
		t.Fatalf("got %+v", got)
	}
	// A charting hits set is wanted as itself.
	got, _ = honestSearch{cs}.Search(context.Background(), "album", "Greatest Hits", "Queen")
	if len(got) != 5 {
		t.Fatalf("hits set dropped: %+v", got)
	}
}

// A song out this month that shares artwork with a 5,000-day-old album is
// shown as its single (or its artist), never as that album; an old song on
// its old album stays.
func TestNewSongOnOldAlbumIsRechecked(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -5000).Format("2006-01-02")
	fresh := now.AddDate(0, 0, -30).Format("2006-01-02")
	dates := map[string]string{fakeID("album|Old Album|A"): old, fakeID("album|New Song|A"): fresh, fakeID("album|Old Hit|A"): old}
	singles := map[string]bool{"New Song": true}
	look := func(kind, name, artist string) (string, string, bool) {
		if kind == "artist" {
			return fakeID("artist|" + name), "", true
		}
		if name != "Old Album" && !singles[name] {
			return "", "", false
		}
		return fakeID("album|" + name + "|" + artist), fakeID("artist|" + artist), true
	}
	var logged []string
	in := Inputs{Now: now, Lookup: look, Date: func(id string) string { return dates[id] },
		Rematch: func(song, artist, from, to string) { logged = append(logged, song+">"+to) },
		Lists: map[string][]Entry{
			"albums:us": {{Rank: 1, Title: "Old Album (Remastered 2011)", Artist: "A", Image: "img1", ReleaseDate: "2011-01-01"}},
			"songs:us": {
				{Rank: 1, Title: "New Song", Artist: "A", Image: "img1", ReleaseDate: fresh},
				{Rank: 2, Title: "Other New", Artist: "A", Image: "img1", ReleaseDate: fresh},
				{Rank: 3, Title: "Old Hit", Artist: "A", Image: "img1", ReleaseDate: old},
			},
		}}
	sh := Build("us", in)
	top := sh.Blocks["top_now"]
	byRank := map[int]Item{}
	for _, it := range top {
		byRank[it.Rank] = it
	}
	if s := byRank[1]; s.Kind != "album" || s.Title != "New Song" || s.Date != fresh || s.Song != "New Song" {
		t.Fatalf("new song not its single: %+v", s)
	}
	if a := sh.Blocks["albums_week"]; len(a) != 1 || a[0].Date != old || a[0].Title != "Old Album" {
		t.Fatalf("album chart item %+v (want MusicBrainz date, clean title)", a)
	}
	for _, it := range top {
		if it.Kind == "album" && it.Date == old && it.Song != "Old Hit" {
			t.Fatalf("new song %q shown as the old album", it.Song)
		}
	}
	once := map[string]bool{}
	for _, l := range logged {
		once[l] = true
	}
	// (The service logs each once; a build tells it every time.)
	if len(once) != 2 || !once["New Song>its single"] || !once["Other New>its artist"] {
		t.Fatalf("re-matches %v", logged)
	}
}

func TestTileItemsCarryReleaseDates(t *testing.T) {
	songs, _ := ParsePlaylist(strings.NewReader(string(readFixture(t, "playlist.json"))))
	look := func(kind, name, artist string) (string, string, bool) {
		if kind == "artist" {
			return fakeID("artist|" + name), "", true
		}
		return fakeID("album|" + name + "|" + artist), fakeID("artist|" + artist), true
	}
	in := Inputs{Now: time.Now(), Lookup: look, Date: func(string) string { return "2026-10-10" }, Lists: map[string][]Entry{"list:pop:0": songs}}
	for _, it := range Build("us", in).Blocks["tile:pop"] {
		if it.Kind == "album" && it.Date != "2026-10-10" {
			t.Fatalf("tile album without its date: %+v", it)
		}
	}
}
