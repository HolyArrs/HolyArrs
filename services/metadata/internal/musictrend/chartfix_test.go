package musictrend

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

type fixLists struct{ entries []Entry }

func (f fixLists) ChartList(_ context.Context, region, kind string) (ListChart, bool) {
	if region != "fr" || kind != "albums" {
		return ListChart{}, false
	}
	return ListChart{Entries: f.entries}, true
}
func (fixLists) GenreTiles() []GenreTile { return nil }
func (fixLists) Regions() []string       { return []string{"fr"} }
func (fixLists) Built() time.Time        { return time.Time{} }

func TestSplitReleaseTag(t *testing.T) {
	for in, want := range map[string][2]string{
		"BREAK EVEN - EP":             {"BREAK EVEN", "ep"},
		"Big One - Single":            {"Big One", "single"},
		"WpointM - 93120 - EP":        {"WpointM - 93120", "ep"},
		"Plain":                       {"Plain", ""},
		" - EP":                       {" - EP", ""},
		"Goat Season (Part Two) - EP": {"Goat Season (Part Two)", "ep"},
	} {
		got, typ := SplitReleaseTag(in)
		if got != want[0] || typ != want[1] {
			t.Errorf("%q: got %q %q, want %q %q", in, got, typ, want[0], want[1])
		}
	}
}

func TestFirstArtist(t *testing.T) {
	for in, want := range map[string]string{
		"Drake & 21 Savage":               "Drake",
		"Fuerza Regida & Grupo Frontera":  "Fuerza Regida",
		"Anirudh Ravichander, Heisenberg": "Anirudh Ravichander",
		"Ozuna feat. Cardi B":             "Ozuna",
		"Bruno X Spacc":                   "Bruno",
		"Solo":                            "Solo",
		"AC/DC":                           "AC/DC",
	} {
		if got := FirstArtist(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// The served chart: explicit when any source says so, Apple's " - EP" tag
// as its own field, the first artist of a credit linked, albums matched in
// the slim copy by artist and title, one row per album, pre-orders marked.
func TestChartFixes(t *testing.T) {
	s := chartService(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	// Stored matches: the first artist of a credit, never the whole credit.
	_, _ = s.db.Exec("INSERT INTO matches VALUES ('artist','Aya Nakamura','',?, '', ?)", singer, now.Unix())
	_, _ = s.db.Exec("INSERT INTO matches VALUES ('artist','Drake','',?, '', ?)", artistA, now.Unix())
	s.SetSlimAlbum(func(_ context.Context, artist, title string) string {
		if artist == artistA && title == "Her Loss" {
			return groupA
		}
		return ""
	})
	// Apple knows the album is explicit, in another storefront.
	s.apple["be"] = chart{albums: []Entry{{Name: "Destinée Supremacy", Artist: "Aya Nakamura", Explicit: true}}}
	s.apple["fr"] = chart{albums: []Entry{{Name: "BREAK EVEN - EP", Artist: "ILLIT"}}}
	s.SetLists(fixLists{entries: []Entry{
		{Name: "Destinée Supremacy", Artist: "Aya Nakamura", Date: "2026-10-01"},
		{Name: "Her Loss", Artist: "Drake & 21 Savage", Date: "2022-11-04"},
		{Name: "Destinee Supremacy", Artist: "Aya Nakamura", Date: "2026-10-01"},
		{Name: "Grand Theft Auto VI: The Album", Artist: "Various Artists", Date: "2026-11-19"},
	}})
	got, ok := s.Charts(ctx, "FR", "albums", "", now)
	if !ok || len(got) != 3 {
		t.Fatalf("want 3 rows (duplicate dropped), got %+v", got)
	}
	if !got[0].Explicit || got[0].ArtistMBID != singer {
		t.Fatalf("explicit from Apple, artist linked: %+v", got[0])
	}
	if got[1].ArtistMBID != artistA || got[1].MBID != groupA {
		t.Fatalf("first artist and slim album link: %+v", got[1])
	}
	if got[2].Rank != 3 || !got[2].PreOrder || got[0].PreOrder {
		t.Fatalf("pre-order flag and ranks: %+v", got)
	}
	// Apple's own chart: the EP tag leaves the title for its own field.
	s.SetLists(nil)
	ap, ok := s.Charts(ctx, "FR", "albums", "", now)
	if !ok || ap[0].Title != "BREAK EVEN" || ap[0].ReleaseType != "ep" {
		t.Fatalf("apple title %+v", ap)
	}
}

func TestAppleEntriesStripReleaseTag(t *testing.T) {
	var it appleChartItem
	it.Attributes.Name, it.Attributes.Artist = "Still In Charge - EP", "BNXN"
	es := appleEntries([]appleChartItem{it}, map[string]bool{})
	if len(es) != 1 || es[0].Name != "Still In Charge" || es[0].Type != "ep" {
		t.Fatalf("%+v", es)
	}
}

// A day kept before the fix loads without Apple's EP tag in its titles.
func TestKeptChartLoadsWithoutReleaseTag(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.sqlite")
	s, err := Open(p, Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	s.keepRanks("fr", chart{albums: []Entry{{Name: "MOR 6FM - EP", Artist: "Mor"}}}, time.Now())
	s.Close()
	s, err = Open(p, Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if a := s.apple["fr"].albums; len(a) != 1 || a[0].Name != "MOR 6FM" || a[0].Type != "ep" {
		t.Fatalf("%+v", a)
	}
}
