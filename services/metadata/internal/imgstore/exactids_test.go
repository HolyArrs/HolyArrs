package imgstore

import (
	"context"
	"testing"
)

// viaLog records how each match was found.
func viaLog(g *rig) map[string]string {
	got := map[string]string{}
	g.r.Via = func(kind, mbid, source, via string) { got[source+":"+mbid] = via }
	return got
}

const wdLink = "https://www.wikidata.org/wiki/Q5"

// Apple: no link, a barcode resolves to one Apple artist: that is the match
// (found by barcode), ahead of a name search.
func TestAppleBarcodeBeforeName(t *testing.T) {
	g := newRig(t)
	keepIn(g)
	via := viaLog(g)
	g.artist("Roxana")
	g.r.Barcodes = func(context.Context, string, int) []string { return []string{"5012345678900"} }
	g.f.set("/v1/catalog/us/albums", map[string]any{"data": []any{map[string]any{"id": "a1",
		"relationships": map[string]any{"artists": map[string]any{"data": []any{map[string]any{"id": "900", "attributes": map[string]any{"name": "Roxana"}}}}}}}})
	g.f.set("/v1/catalog/us/artists/900", map[string]any{"data": []any{map[string]any{"id": "900", "attributes": map[string]any{"name": "Roxana"}}}})
	g.f.set("/v1/catalog/us/search?Roxana", map[string]any{"results": map[string]any{"artists": map[string]any{"data": []any{
		map[string]any{"id": "777", "attributes": map[string]any{"name": "Roxana"}}}}}})
	if _, id, err := g.r.AppleArtist(context.Background(), artistID); err != nil || id != "900" {
		t.Fatalf("barcode match %q %v", id, err)
	}
	if via["applemusic:"+artistID] != "barcode" || g.f.hit("/v1/catalog/us/search") != 0 {
		t.Fatalf("via %v, searches %d", via, g.f.hit("/v1/catalog/us/search"))
	}
}

// Apple: no barcode, the MusicBrainz-linked Wikidata item names the Apple id.
func TestAppleWikidataBeforeName(t *testing.T) {
	g := newRig(t)
	keepIn(g)
	via := viaLog(g)
	g.artist("Roxana", wdLink)
	g.r.WikidataIDs = func(_ context.Context, qid string) (string, string, string, error) {
		if qid != "Q5" {
			t.Fatalf("asked %q", qid)
		}
		return "901", "", "", nil
	}
	g.f.set("/v1/catalog/us/artists/901", map[string]any{"data": []any{map[string]any{"id": "901", "attributes": map[string]any{"name": "Roxana"}}}})
	if _, id, err := g.r.AppleArtist(context.Background(), artistID); err != nil || id != "901" || via["applemusic:"+artistID] != "wikidata" {
		t.Fatalf("wikidata match %q %v %v", id, err, via)
	}
}

// Deezer: barcodes that point to two different artists prove nothing; one
// artist across them is the match; a MusicBrainz link is still first.
func TestDeezerBarcodes(t *testing.T) {
	ctx := context.Background()
	setup := func(codes ...string) (*rig, map[string]string) {
		g, mb, _ := matchRig(t)
		via := viaLog(g)
		mb["artist:"+artistID] = map[string]any{"name": "Lucy"}
		g.r.Barcodes = func(context.Context, string, int) []string { return codes }
		g.f.set("/deezer/album/upc:10000001", map[string]any{"id": 10, "artist": map[string]any{"id": 55}})
		g.f.set("/deezer/album/upc:10000002", map[string]any{"id": 11, "artist": map[string]any{"id": 55}})
		g.f.set("/deezer/album/upc:10000003", map[string]any{"id": 12, "artist": map[string]any{"id": 66}})
		g.f.set("/deezer/album/upc:10000004", map[string]any{"error": map[string]any{"type": "DataException", "code": 800}})
		g.f.set("/deezer/search/artist?Lucy", map[string]any{"data": []any{}})
		return g, via
	}
	g, via := setup("10000004", "10000001", "10000002")
	if id, how, err := g.r.DeezerMatch(ctx, KindArtist, artistID); err != nil || id != "55" || how != MatchBarcode || via["deezer:"+artistID] != "barcode" {
		t.Fatalf("barcode match %q %q %v %v", id, how, err, via)
	}
	g, _ = setup("10000001", "10000003")
	if id, how, _ := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "" || how != "" {
		t.Fatalf("conflicting barcodes taken: %q %q", id, how)
	}
	g, _ = setup("10000001")
	g.r.MB = mbStore{"artist:" + artistID: map[string]any{"name": "Lucy", "relations": []any{rel("https://www.deezer.com/artist/70")}}}.get
	g.f.set("/deezer/artist/70", map[string]any{"id": 70, "name": "Lucy"})
	if id, how, _ := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "70" || how != MatchLink {
		t.Fatalf("link not first: %q %q", id, how)
	}
}

// Spotify: a barcode search names the album's first artist; Wikidata is
// next; the name search last.
func TestSpotifyBarcodeThenWikidata(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	keepIn(g)
	via := viaLog(g)
	g.artist("Lucy")
	g.r.Barcodes = func(context.Context, string, int) []string { return []string{"90000009"} }
	g.f.set("/spotify-auth", map[string]any{"access_token": "t", "expires_in": 3600})
	g.f.set("/spotify/v1/search?upc:90000009", map[string]any{"albums": map[string]any{"items": []any{map[string]any{"artists": []any{map[string]any{"id": "spBBBBBBBBBBBBBBBBBBBB", "name": "Lucy"}}}}}})
	g.f.set("/spotify/v1/artists/spBBBBBBBBBBBBBBBBBBBB", map[string]any{"id": "spBBBBBBBBBBBBBBBBBBBB", "name": "Lucy"})
	if id, err := g.r.SpotifyArtist(ctx, artistID); err != nil || id != "spBBBBBBBBBBBBBBBBBBBB" || via["spotify:"+artistID] != "barcode" {
		t.Fatalf("barcode %q %v %v", id, err, via)
	}
	g = newRig(t)
	keepIn(g)
	via = viaLog(g)
	g.artist("Lucy", wdLink)
	g.r.WikidataIDs = func(context.Context, string) (string, string, string, error) {
		return "", "", "spWWWWWWWWWWWWWWWWWWWW", nil
	}
	g.f.set("/spotify-auth", map[string]any{"access_token": "t", "expires_in": 3600})
	g.f.set("/spotify/v1/artists/spWWWWWWWWWWWWWWWWWWWW", map[string]any{"id": "spWWWWWWWWWWWWWWWWWWWW", "name": "Lucy"})
	if id, err := g.r.SpotifyArtist(ctx, artistID); err != nil || id != "spWWWWWWWWWWWWWWWWWWWW" || via["spotify:"+artistID] != "wikidata" {
		t.Fatalf("wikidata %q %v %v", id, err, via)
	}
}

// Crowd-sourced values (Wikidata, MusicBrainz barcodes) are checked before
// use: a bad barcode is never asked, a bad id never taken.
func TestExactIDRefusesMaliciousValues(t *testing.T) {
	ctx := context.Background()
	bad := []string{"../1", "1?x=2", "1#frag", "1/2", "12 34", "", "abc"}
	for _, v := range bad {
		g, mb, _ := matchRig(t)
		mb["artist:"+artistID] = map[string]any{"name": "Lucy"}
		g.r.Barcodes = func(context.Context, string, int) []string { return []string{v} }
		g.r.WikidataIDs = func(context.Context, string) (string, string, string, error) {
			return v, v, v + "AAAAAAAAAAAAAAAAAAAAA", nil
		}
		for _, src := range []string{"applemusic", "deezer", "spotify"} {
			if id, _, _, err := g.r.ExactID(ctx, src, artistID, "Lucy", "Q5", ""); id != "" || err != nil {
				t.Fatalf("%s took %q: %q %v", src, v, id, err)
			}
		}
		if n := g.f.hits["/deezer/album/upc:"+v]; n != 0 {
			t.Fatalf("bad barcode %q asked", v)
		}
	}
	if !ValidSourceID("spotify", "0OjRa6r1EIPF5RZPy1SGz9") || ValidSourceID("spotify", "short") || !ValidSourceID("deezer", "67898492") || ValidSourceID("applemusic", "12a") {
		t.Fatal("ValidSourceID")
	}
	if !ValidBarcode("5012345678900") || ValidBarcode("1234567") || ValidBarcode("123456789012345") {
		t.Fatal("ValidBarcode")
	}
}

// A barcode names the artist only through a name: of the album's artists,
// the one with the artist's name counts. Without a name match (another
// script, a member of a duo, an alias) two barcodes must agree.
func TestBarcodeNeedsNameOrTwoBarcodes(t *testing.T) {
	ctx := context.Background()
	setup := func(name string, codes ...string) *rig {
		g, mb, _ := matchRig(t)
		mb["artist:"+artistID] = map[string]any{"name": name}
		g.r.Barcodes = func(context.Context, string, int) []string { return codes }
		// A duo's album lists its members: GD&TOP's barcode names G-DRAGON first.
		g.f.set("/deezer/album/upc:10000001", map[string]any{"artist": map[string]any{"id": 1, "name": "G-DRAGON"},
			"contributors": []any{map[string]any{"id": 1, "name": "G-DRAGON"}, map[string]any{"id": 2, "name": "T.O.P"}}})
		g.f.set("/deezer/album/upc:10000002", map[string]any{"artist": map[string]any{"id": 3, "name": "Sherine"},
			"contributors": []any{map[string]any{"id": 3, "name": "Sherine"}}})
		g.f.set("/deezer/album/upc:10000003", map[string]any{"artist": map[string]any{"id": 3, "name": "Sherine"}})
		g.f.set("/deezer/album/upc:10000004", map[string]any{"artist": map[string]any{"id": 4, "name": "Guest"},
			"contributors": []any{map[string]any{"id": 4, "name": "Guest"}, map[string]any{"id": 5, "name": "T.O.P"}}})
		g.f.set("/deezer/search/artist?"+name, map[string]any{"data": []any{}})
		return g
	}
	cases := []struct {
		name  string
		codes []string
		want  string
	}{
		{"GD&TOP", []string{"10000001"}, ""},             // no album artist of that name
		{"شيرين", []string{"10000002"}, ""},              // another script, one barcode
		{"شيرين", []string{"10000002", "10000003"}, "3"}, // two barcodes agree
		{"T.O.P", []string{"10000004"}, "5"},             // the named one among the album's artists
	}
	for _, c := range cases {
		id, _, _ := setup(c.name, c.codes...).r.DeezerMatch(ctx, KindArtist, artistID)
		if id != c.want {
			t.Errorf("%s %v: %q, want %q", c.name, c.codes, id, c.want)
		}
	}
}
