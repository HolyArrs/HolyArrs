package musictags

import (
	"testing"
	"time"
)

var testRecipes = []Recipe{
	{Key: "genre:rock", Name: "Rock", Kind: KindGenre, Tags: map[string]float64{"rock": 1}},
	{Key: "genre:pop", Name: "Pop", Kind: KindGenre, Tags: map[string]float64{"pop": 1}, ChartGenres: []string{"Pop"}},
	{Key: "decade:1990s", Name: "90s", Kind: KindDecade, From: 1990, To: 1999},
	{Key: "mood:chill", Name: "Chill", Kind: KindMood, Tags: map[string]float64{"chill": 1}, Own: []string{"chill"}},
}

func testSource() *Source {
	src := NewSource()
	src.Album(Album{ID: "rg1", Title: "Loud", Artist: "Band", ArtistID: "ar1", Date: "1994-01-01", Type: "Album", Tags: tags("rock", 5, "a", 1, "b", 1), Listens: 1000, Listeners: 100, Art: true})
	src.Album(Album{ID: "rg2", Title: "Soft", Artist: "Singer", ArtistID: "ar2", Date: "2024-01-01", Type: "Album", Tags: tags("pop", 3, "chill", 3, "c", 1), Listens: 50, Listeners: 40, Art: true})
	src.Album(Album{ID: "rg3", Title: "No Cover", Artist: "Band", ArtistID: "ar1", Date: "1995-01-01", Type: "Album", Tags: tags("rock", 5, "a", 1, "b", 1), Art: false})
	src.Chart("us", "rg2", 1, "Pop")
	src.Chart("gb", "rg2", 3, "Pop")
	src.Chart("gb", "rg1", 2, "Rock")
	src.Artist(Artist{ID: "ar1", Name: "Band", Tags: tags("rock", 9), Listens: 5000, Art: true})
	return src
}

func TestBuildIndexesItemsByKey(t *testing.T) {
	b := Build(testSource(), testRecipes, time.Date(2026, 10, 24, 6, 30, 0, 0, time.UTC))
	idx := map[string]int{}
	for i, it := range b.Items {
		idx[it.Kind+":"+it.ID] = i
	}
	if _, ok := idx["a:rg3"]; ok {
		t.Fatal("album without a cover is in the bundle")
	}
	has := func(key, id string) (KeyMember, bool) {
		for _, m := range b.Keys[key] {
			if m.I == idx[id] {
				return m, true
			}
		}
		return KeyMember{}, false
	}
	if _, ok := has("genre:rock", "a:rg1"); !ok {
		t.Fatal("rg1 not in rock")
	}
	if _, ok := has("decade:1990s", "a:rg1"); !ok {
		t.Fatal("rg1 not in the 90s")
	}
	if m, ok := has("mood:chill", "a:rg2"); !ok || !m.Own {
		t.Fatalf("rg2 chill = %v, %v; want own member", m, ok)
	}
	if _, ok := has("genre:rock", "r:ar1"); !ok {
		t.Fatal("artist ar1 not in rock")
	}
	if b.Names["genre:rock"].Name != "Rock" {
		t.Fatalf("names = %v", b.Names["genre:rock"])
	}
}

func TestBuildChartSignals(t *testing.T) {
	b := Build(testSource(), testRecipes, time.Now())
	var rg1, rg2 BItem
	for _, it := range b.Items {
		switch it.ID {
		case "rg1":
			rg1 = it
		case "rg2":
			rg2 = it
		}
	}
	if rg2.C["us"] != 1 || rg2.C["gb"] != 3 {
		t.Fatalf("rg2 ranks = %v", rg2.C)
	}
	// rg2 is on two charts, rg1 on one: the many-country score ranks rg2 first.
	if rg2.CW != 1 || rg1.CW != 2 {
		t.Fatalf("CW = %d, %d; want 1, 2", rg2.CW, rg1.CW)
	}
	if rg2.P != 1000 {
		t.Fatalf("P = %d, want 1000 (rank 1)", rg2.P)
	}
}

func TestSourceAccessors(t *testing.T) {
	src := testSource()
	if src.ArtistName("AR1") != "Band" || src.ArtistName("nobody") != "" {
		t.Fatal("ArtistName")
	}
	if got := src.AlbumIDs(); len(got) != 3 || got[0] != "rg1" {
		t.Fatalf("AlbumIDs = %v", got)
	}
}

func TestChartGainSignals(t *testing.T) {
	src := testSource()
	src.ChartGain("gb", "rg1", 0.4)
	src.ChartGain("us", "rg1", 0.1) // not on the us chart: ignored
	b := Build(src, testRecipes, time.Now())
	for _, it := range b.Items {
		if it.ID == "rg1" {
			if it.M != 400 || it.G["gb"] != 400 || len(it.G) != 1 {
				t.Fatalf("M = %d, G = %v", it.M, it.G)
			}
			return
		}
	}
	t.Fatal("rg1 missing")
}

func TestArtistsTakeTagsFromTheirAlbums(t *testing.T) {
	src := NewSource()
	src.Album(Album{ID: "a1", Title: "X", Artist: "Band", ArtistID: "ar9", Tags: tags("rock", 2, "a", 1, "b", 1), ArtistTags: tags("rock", 7), Listens: 900, Art: true})
	src.Artist(Artist{ID: "ar9", Name: "Band", Listens: 900, Art: true})
	b := Build(src, testRecipes, fixedNow)
	for _, m := range b.Keys["genre:rock"] {
		if b.Items[m.I].Kind == "r" {
			return
		}
	}
	t.Fatal("artist without its own tags is not in its albums' genre")
}

func TestObscureFreshAlbumsAreLeftOut(t *testing.T) {
	src := NewSource()
	src.Album(Album{ID: "quiet", Title: "Quiet", Date: "2026-10-01", Tags: tags("rock", 1), Listens: 12, Art: true})
	src.Album(Album{ID: "heard", Title: "Heard", Date: "2026-10-01", Tags: tags("rock", 1), Listens: 5000, Art: true})
	src.Album(Album{ID: "charting", Title: "Charting", Date: "2026-10-01", Tags: tags("rock", 1), Art: true})
	src.Chart("us", "charting", 9)
	b := Build(src, testRecipes, fixedNow)
	ids := map[string]bool{}
	for _, it := range b.Items {
		ids[it.ID] = true
	}
	if ids["quiet"] || !ids["heard"] || !ids["charting"] {
		t.Fatalf("items = %v; want heard and charting, not quiet", ids)
	}
}

func TestThrowbackIsOldAndWellPlayed(t *testing.T) {
	src := NewSource()
	for i, y := range []string{"1985", "1990", "1995", "2001", "2024"} {
		src.Album(Album{ID: "t" + y, Title: y, Date: y + "-01-01", Tags: tags("rock", 1), Listens: int64(1000 * (i + 1)), Art: true})
	}
	src.Album(Album{ID: "rare", Title: "Rare", Date: "1980-01-01", Tags: tags("rock", 1), Listens: 300, Art: true})
	rs := []Recipe{{Key: "mood:throwback", Name: "Throwback", Kind: KindMood, OlderThan: 15, TopShare: 0.6}}
	b := Build(src, rs, fixedNow)
	in := map[string]bool{}
	for _, m := range b.Keys["mood:throwback"] {
		in[b.Items[m.I].ID] = true
	}
	if !in["t2001"] || !in["t1995"] || in["t2024"] || in["rare"] {
		t.Fatalf("throwback = %v", in)
	}
}

func TestBuildVersionIsContentHash(t *testing.T) {
	now := time.Date(2026, 10, 24, 6, 30, 0, 0, time.UTC)
	a := Build(testSource(), testRecipes, now)
	b := Build(testSource(), testRecipes, now.Add(time.Hour))
	if a.Version != b.Version {
		t.Fatal("version changed with only the build time")
	}
	src := testSource()
	src.Chart("de", "rg1", 5, "Rock")
	c := Build(src, testRecipes, now)
	if c.Version == a.Version {
		t.Fatal("version kept although a chart rank changed")
	}
}
