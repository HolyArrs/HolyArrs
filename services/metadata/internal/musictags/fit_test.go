package musictags

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func tags(kv ...any) []Tag {
	var out []Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, Tag{Name: kv[i].(string), Votes: kv[i+1].(int)})
	}
	return out
}

func TestFitWeightsAlbumAndArtistTags(t *testing.T) {
	r := Recipe{Key: "genre:rock", Kind: KindGenre, Tags: map[string]float64{"rock": 1, "hard rock": 0.6}}
	it := Item{AlbumTags: tags("rock", 4, "hard rock", 2, "loud", 4), ArtistTags: tags("rock", 10)}
	// album: rock 4/4·1 + hard rock 2/4·0.6 = 1.3 → profile 0.65·1.3; artist: rock 1 → 0.35·1.
	got, own := Fit(it, r)
	if !near(got, 1) || own {
		t.Fatalf("Fit = %v, %v; want 1 (clamped), not own", got, own)
	}
	it = Item{AlbumTags: tags("pop", 4, "rock", 1, "x", 1), ArtistTags: tags("pop", 10)}
	got, _ = Fit(it, r)
	if want := 0.65 * 0.25; !near(got, want) {
		t.Fatalf("Fit = %v, want %v", got, want)
	}
}

func TestFitArtistTagsLeadWhenAlbumHasFewTags(t *testing.T) {
	r := Recipe{Key: "genre:jazz", Kind: KindGenre, Tags: map[string]float64{"jazz": 1}}
	it := Item{AlbumTags: tags("live", 1), ArtistTags: tags("jazz", 8, "bebop", 4)}
	got, _ := Fit(it, r)
	if want := 0.6; !near(got, want) {
		t.Fatalf("Fit = %v, want %v (artist weight 0.6 under 3 album tags)", got, want)
	}
}

func TestFitMinVotesIgnoresThinTags(t *testing.T) {
	r := Recipe{Key: "mood:sad", Kind: KindMood, Tags: map[string]float64{"sad": 1}, MinVotes: 2}
	it := Item{AlbumTags: tags("sad", 1, "pop", 1, "ballad", 1)}
	if got, _ := Fit(it, r); got != 0 {
		t.Fatalf("Fit = %v, want 0 (one vote under MinVotes 2)", got)
	}
}

func TestFitExcludeDropsItem(t *testing.T) {
	r := Recipe{Key: "mood:chill", Kind: KindMood, Tags: map[string]float64{"chillout": 1}, Exclude: []string{"metal"}}
	it := Item{AlbumTags: tags("chillout", 2, "metal", 4, "x", 1)}
	if got, _ := Fit(it, r); got != 0 {
		t.Fatalf("Fit = %v, want 0 (exclude tag at full strength)", got)
	}
}

func TestFitTitleWordAddsHalfAndCountsAsOwn(t *testing.T) {
	r := Recipe{Key: "season:christmas", Kind: KindSeason, Tags: map[string]float64{"pop": 0.3}, Words: []string{"christmas", "navidad"}}
	it := Item{Title: "Merry Christmas II You", AlbumTags: tags("pop", 2, "a", 1, "b", 1)}
	got, own := Fit(it, r)
	if want := 0.3 + 0.5; !near(got, want) || !own { // no artist tags: album tags count fully
		t.Fatalf("Fit = %v, %v; want %v, own", got, own, want)
	}
	// Whole words only: "Christmastime" still matches as a prefix word, "xchristmas" does not.
	if _, own := Fit(Item{Title: "Xchristmas"}, r); own {
		t.Fatal("inner match counted")
	}
}

func TestFitOwnTagMarksTaggedBasis(t *testing.T) {
	r := Recipe{Key: "mood:chill", Kind: KindMood, Tags: map[string]float64{"chill": 1, "downtempo": 0.6}, Own: []string{"chill"}}
	if _, own := Fit(Item{AlbumTags: tags("downtempo", 3, "a", 1, "b", 1)}, r); own {
		t.Fatal("style-only match marked own")
	}
	if _, own := Fit(Item{AlbumTags: tags("chill", 2, "a", 1, "b", 1)}, r); !own {
		t.Fatal("own tag not marked")
	}
}

func TestDecadeMembership(t *testing.T) {
	r := Recipe{Key: "decade:1990s", Kind: KindDecade, From: 1990, To: 1999}
	if got, _ := Fit(Item{Date: "1994-05-10"}, r); got != 1 {
		t.Fatalf("1994 in 1990s = %v", got)
	}
	if got, _ := Fit(Item{Date: "2001"}, r); got != 0 {
		t.Fatalf("2001 in 1990s = %v", got)
	}
	if got, _ := Fit(Item{}, r); got != 0 {
		t.Fatalf("no date in 1990s = %v", got)
	}
}

func TestGenreKeepsOnlyNearBestGenre(t *testing.T) {
	rs := []Recipe{
		{Key: "genre:alternative", Kind: KindGenre, Tags: map[string]float64{"alternative rock": 1}, MinT: 0.5},
		{Key: "genre:pop", Kind: KindGenre, Tags: map[string]float64{"art pop": 1}, MinT: 0.5},
	}
	it := Item{AlbumTags: tags("alternative rock", 4, "art pop", 4, "x", 1), ArtistTags: tags("alternative rock", 10, "art pop", 1)}
	got := Members(it, rs)
	if _, ok := got["genre:alternative"]; !ok {
		t.Fatal("best genre missing")
	}
	if _, ok := got["genre:pop"]; ok {
		t.Fatalf("pop kept at %v, under 0.75 of the best genre", got["genre:pop"])
	}
}

func TestChartGenreCountsForUntaggedItems(t *testing.T) {
	r := Recipe{Key: "genre:hip-hop", Kind: KindGenre, Tags: map[string]float64{"hip hop": 1}, ChartGenres: []string{"Hip-Hop/Rap"}, MinT: 0.5}
	got := Members(Item{ChartGenres: []string{"Hip-Hop/Rap"}}, []Recipe{r})
	if m, ok := got["genre:hip-hop"]; !ok || m.T != 1 {
		t.Fatalf("chart genre membership = %v, %v", m, ok)
	}
}
