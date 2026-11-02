package musictrend

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"
)

// Apple chart rows keep the artist's Apple id (from its link) and the
// album's barcode (its upc, else the artwork's file name).
func TestAppleEntriesKeepArtistIDAndBarcode(t *testing.T) {
	var a, b appleChartItem
	a.Attributes.Name, a.Attributes.Artist = "Album", "Basta"
	a.Attributes.ArtistURL = "https://music.apple.com/ru/artist/basta/161441806"
	a.Attributes.UPC = "4601234567890"
	b.Attributes.Name, b.Attributes.Artist = "Other", "Omar"
	b.Attributes.Artwork.URL = "https://is1-ssl.mzstatic.com/image/thumb/Music211/v4/47/61/88/x/885288695823.jpg/{w}x{h}bb.{f}"
	es := appleEntries([]appleChartItem{a, b}, map[string]bool{})
	if es[0].AppleArtist != "161441806" || es[0].UPC != "4601234567890" {
		t.Fatalf("a %+v", es[0])
	}
	if es[1].AppleArtist != "" || EntryBarcode(es[1]) != "885288695823" {
		t.Fatalf("b %+v %q", es[1], EntryBarcode(es[1]))
	}
	// A kept day has no upc: the cover's file name is read.
	if EntryBarcode(Entry{Cover: "https://x/885288695823.jpg/500x500bb.jpg"}) != "885288695823" || EntryBarcode(Entry{Cover: "https://x/cover.jpg/500x500bb.jpg"}) != "" {
		t.Fatal("cover barcode")
	}
}

// An Apple chart row's own artist id, or its barcode, resolved exactly,
// beats the name match; an album then loses a name-matched id of another
// artist; unresolved rows keep the name match.
func TestChartFromUsesTheRowsArtistID(t *testing.T) {
	s, err := Open(t.TempDir()+"/m.sqlite", Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const exact = "e1000000-0000-4000-8000-000000000001"
	var asked [][]string
	s.SetExactArtist(func(_ context.Context, source, id, _ string, codes []string) string {
		asked = append(asked, append([]string{source, id}, codes...))
		if id == "161441806" || len(codes) > 0 && codes[0] == "885288695823" {
			return exact
		}
		return ""
	})
	es := []Entry{{Name: "Album", Artist: "Basta", AppleArtist: "161441806"},
		{Name: "Other", Artist: "Omar", Cover: "https://x/885288695823.jpg/500x500bb.jpg"},
		{Name: "Third", Artist: "Nobody"}}
	out := s.chartFrom(context.Background(), "ru", "albums", "", es, false, time.Now())
	if out[0].ArtistMBID != exact || out[1].ArtistMBID != exact || out[2].ArtistMBID == exact {
		t.Fatalf("items %+v", out)
	}
	if len(asked) != 3 || asked[0][0] != "applemusic" || asked[0][1] != "161441806" {
		t.Fatalf("asked %v", asked)
	}
}

// Apple's artist lists are built from its song and album rows: each artist
// keeps its Apple id and its albums' barcodes, and the published list
// takes the exact resolution over the name.
func TestAppleArtistListUsesExactArtist(t *testing.T) {
	es := RankArtists([]Entry{{Name: "S", Artist: "Basta", Cover: "https://x/4601234567890.jpg/500x500bb.jpg"}},
		[]Entry{{Name: "A", Artist: "Basta", AppleArtist: "161441806", UPC: "4601234567891"}})
	if len(es) != 1 || es[0].AppleArtist != "161441806" || len(es[0].Codes) != 2 {
		t.Fatalf("ranked %+v", es)
	}
	s, err := Open(t.TempDir()+"/m.sqlite", Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const exact = "e1000000-0000-4000-8000-000000000001"
	s.SetExactArtist(func(_ context.Context, source, id, _ string, codes []string) string {
		if source == "applemusic" && id == "161441806" && len(codes) == 2 {
			return exact
		}
		return ""
	})
	e, ok := s.resolve(context.Background(), "artist", es[0], false)
	if !ok || e.MBID != exact {
		t.Fatalf("resolved %+v %v", e, ok)
	}
}

// "Never show the wrong artist": without exact evidence, a name several
// artists carry links to none of them (the row stays, as plain text); a
// name only one artist carries keeps its link.
func TestSharedNameWithoutEvidenceLinksNoArtist(t *testing.T) {
	s, err := Open(t.TempDir()+"/m.sqlite", Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const byName = "b1000000-0000-4000-8000-000000000001"
	if _, err := s.db.Exec(`INSERT INTO matches VALUES('artist','Basta','',?,'',1),('artist','Unique','',?,'',1)`, byName, byName); err != nil {
		t.Fatal(err)
	}
	s.SetExactArtist(func(context.Context, string, string, string, []string) string { return "" })
	s.SetSharedName(func(_ context.Context, name string) bool { return name == "Basta" })
	ctx := context.Background()
	out := s.items(ctx, "artist", SrcApple, []Entry{{Name: "Basta"}, {Name: "Unique"}}, false)
	if len(out) != 2 {
		t.Fatalf("items %+v", out)
	}
	if a := out[0].(artistItem); a.MBID != "" || a.Name != "Basta" {
		t.Fatalf("shared name linked: %+v", a)
	}
	if a := out[1].(artistItem); a.MBID != byName {
		t.Fatalf("unique name lost its link: %+v", a)
	}
	got := s.chartFrom(ctx, "lv", "albums", "", []Entry{{Name: "Album", Artist: "Basta"}}, false, time.Now())
	if got[0].ArtistMBID != "" {
		t.Fatalf("album row linked a shared name: %+v", got[0])
	}
}

// After a restart the lists are published once even when no source
// changed, so a new build's resolution shows at once.
func TestFirstRefreshAlwaysPublishes(t *testing.T) {
	var s Service
	if !s.mustPublish(false, false) {
		t.Fatal("first refresh did not publish")
	}
	s.published = true
	if s.mustPublish(false, false) || !s.mustPublish(true, false) || !s.mustPublish(false, true) {
		t.Fatal("later refreshes")
	}
}

// Apple chart rows keep their catalog id and kind.
func TestAppleEntriesKeepCatalogID(t *testing.T) {
	var a appleChartItem
	a.ID, a.Type = "1440833098", "albums"
	a.Attributes.Name, a.Attributes.Artist = "Scorpion", "Drake"
	es := appleEntries([]appleChartItem{a}, map[string]bool{})
	if es[0].AppleID != "1440833098" || es[0].AppleKind != "albums" {
		t.Fatalf("%+v", es[0])
	}
}

// Rows without an artist id get it from Apple's catalog, asked once per
// item in batches (ids=, 50 at a time), kept for good: a second fill asks
// nothing. An item several artists are credited on gets none.
func TestFillAppleArtists(t *testing.T) {
	s, err := Open(t.TempDir()+"/m.sqlite", Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var paths []string
	s.SetApple(func(_ context.Context, path string, q url.Values, out any) (bool, error) {
		paths = append(paths, path+"?"+q.Get("ids"))
		body := `{"data":[]}`
		switch path {
		case "/v1/catalog/is/albums":
			body = `{"data":[{"id":"1","relationships":{"artists":{"data":[{"id":"271256"}]}}},{"id":"2","relationships":{"artists":{"data":[{"id":"271256"},{"id":"999"}]}}}]}`
		case "/v1/catalog/is/songs":
			body = `{"data":[{"id":"3","relationships":{"artists":{"data":[{"id":"183313439"}]}}}]}`
		}
		return true, json.Unmarshal([]byte(body), out)
	})
	es := []Entry{{Name: "A", Artist: "Drake", AppleID: "1", AppleKind: "albums"}, {Name: "B", Artist: "Drake & X", AppleID: "2", AppleKind: "albums"},
		{Name: "C", Artist: "Ed Sheeran", AppleID: "3", AppleKind: "songs"}, {Name: "D", Artist: "Kept", AppleArtist: "5"}}
	ctx := context.Background()
	if err := s.fillAppleArtists(ctx, "is", es); err != nil {
		t.Fatal(err)
	}
	if es[0].AppleArtist != "271256" || es[1].AppleArtist != "" || es[2].AppleArtist != "183313439" || es[3].AppleArtist != "5" {
		t.Fatalf("filled %+v", es)
	}
	if len(paths) != 2 || paths[0] != "/v1/catalog/is/albums?1,2" || paths[1] != "/v1/catalog/is/songs?3" {
		t.Fatalf("asked %v", paths)
	}
	again := []Entry{{Name: "A", Artist: "Drake", AppleID: "1", AppleKind: "albums"}}
	if err := s.fillAppleArtists(ctx, "is", again); err != nil || again[0].AppleArtist != "271256" || len(paths) != 2 {
		t.Fatalf("kept answer asked again: %v %+v", paths, again)
	}
}

// An album matched under one artist whose row resolves exactly to another
// loses that album when the other has none titled like it: a wrong album
// is worse than none.
func TestResolveDropsOtherArtistsAlbum(t *testing.T) {
	s, err := Open(t.TempDir()+"/m.sqlite", Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const homonym, exact, rg = "a1000000-0000-4000-8000-000000000001", "e1000000-0000-4000-8000-000000000001", "r1000000-0000-4000-8000-000000000001"
	if _, err := s.db.Exec(`INSERT INTO matches VALUES('album','Big','Basta',?,?,1)`, rg, homonym); err != nil {
		t.Fatal(err)
	}
	s.SetExactArtist(func(context.Context, string, string, string, []string) string { return exact })
	s.SetSlimAlbum(func(context.Context, string, string) string { return "" })
	e, ok := s.resolve(context.Background(), "album", Entry{Name: "Big", Artist: "Basta", AppleArtist: "1"}, false)
	if ok || e.MBID != "" || e.ArtistMBID != exact {
		t.Fatalf("resolved %+v %v", e, ok)
	}
}

// An edition's title is cut to the album it is filed under; a title that
// names no edition, or a " - " title, has no stem.
func TestEditionStem(t *testing.T) {
	cases := map[string]string{
		"The Great Divide: The Last Of The Bugs":       "", // no edition word: its own album to us
		"folklore: the long pond studio sessions":      "",
		"Ghost Stories: Live 2014":                     "",
		"Teenage Dream: The Complete Confection":       "Teenage Dream",
		"The Life of a Showgirl: The Encore":           "The Life of a Showgirl",
		"SOS Deluxe: LANA":                             "SOS",
		"THE TORTURED POETS DEPARTMENT: THE ANTHOLOGY": "THE TORTURED POETS DEPARTMENT",
		"Born To Die – The Paradise Edition":           "Born To Die",
		"Bad 25th Anniversary":                         "Bad",
		"Dua Lipa - Live From Mexico":                  "",
		"Scorpion":                                     "",
	}
	for in, want := range cases {
		if got := EditionStem(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
