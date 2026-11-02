package imgstore

import (
	"context"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

const rivalID = "d04b1b7f-71a5-4011-9441-d0b5e4122719"

// A kept searched Deezer id loses to the one MusicBrainz links
// (Yorushika: 92503082 kept, 67898492 linked).
func TestDeezerKeptSearchLosesToLink(t *testing.T) {
	g, mb, kept := matchRig(t)
	mb["artist:"+artistID] = map[string]any{"name": "Yorushika", "relations": []any{rel("https://www.deezer.com/artist/67898492")}}
	g.info[KindArtist+":"+artistID] = Subject{Known: true, Name: "Yorushika", Links: []string{"https://www.deezer.com/artist/67898492"}}
	g.f.set("/deezer/artist/67898492", map[string]any{"id": 67898492, "name": "Yorushika"})
	kept[KindArtist+":"+artistID+":deezer"] = keptRow{srctag.Tag{Source: "deezer"}, MatchSearch, "92503082"}
	id, how, err := g.r.DeezerMatch(context.Background(), KindArtist, artistID)
	if err != nil || id != "67898492" || how != MatchLink {
		t.Fatalf("kept search id won over the link: %q %q %v", id, how, err)
	}
}

// A Deezer search winner whose name others carry needs album evidence that
// beats every rival's.
func TestDeezerSearchWithHomonymNeedsEvidence(t *testing.T) {
	ctx := context.Background()
	setup := func(mine, theirs []string) *rig {
		g, mb, _ := matchRig(t)
		g.r.MBTitles = func(_ context.Context, id string) []string {
			if id == rivalID {
				return theirs
			}
			return mine
		}
		g.r.Homonyms = func(context.Context, string, string) []string { return []string{rivalID} }
		mb["artist:"+artistID] = map[string]any{"name": "Lucy"}
		g.f.set("/deezer/search/artist?Lucy", map[string]any{"data": []any{map[string]any{"id": 1, "name": "Lucy", "nb_fan": 5000}}})
		g.f.set("/deezer/artist/1/albums", map[string]any{"data": []any{
			map[string]any{"id": 11, "title": "Panorama"}, map[string]any{"id": 12, "title": "Blue"}, map[string]any{"id": 13, "title": "Dear"}}})
		return g
	}
	if id, _, _ := setup([]string{"Panorama", "Blue", "Childhood"}, []string{"Other"}).r.DeezerMatch(ctx, KindArtist, artistID); id != "1" {
		t.Fatalf("evidence refused: %q", id)
	}
	if id, _, _ := setup([]string{"Panorama"}, []string{"Panorama", "Blue", "Dear"}).r.DeezerMatch(ctx, KindArtist, artistID); id != "" {
		t.Fatalf("taken from a better rival: %q", id)
	}
}

// A Spotify search match: a shared id is a miss, and with same-name rivals
// the albums must fit; MusicBrainz's link beats a kept searched id.
func TestSpotifySearchEvidenceAndLink(t *testing.T) {
	ctx := context.Background()
	setup := func(mine, theirs []string) *rig {
		g := newRig(t)
		keepIn(g)
		g.r.Shared = g.s.SharedID
		g.r.MBTitles = func(_ context.Context, id string) []string {
			if id == rivalID {
				return theirs
			}
			return mine
		}
		g.r.Homonyms = func(context.Context, string, string) []string { return []string{rivalID} }
		g.artist("Lucy")
		g.f.set("/spotify-auth", map[string]any{"access_token": "t", "expires_in": 3600})
		g.f.set("/spotify/v1/search?Lucy", map[string]any{"artists": map[string]any{"items": []any{map[string]any{"id": "sp1", "name": "Lucy"}}}})
		g.f.set("/spotify/v1/artists/sp1/albums", map[string]any{"items": []any{
			map[string]any{"name": "Panorama"}, map[string]any{"name": "Blue"}, map[string]any{"name": "Dear"}}})
		return g
	}
	if id, err := setup([]string{"Panorama", "Blue"}, nil).r.SpotifyArtist(ctx, artistID); err != nil || id != "sp1" {
		t.Fatalf("evidence refused: %q %v", id, err)
	}
	if id, _ := setup([]string{"Panorama"}, []string{"Panorama", "Blue", "Dear"}).r.SpotifyArtist(ctx, artistID); id != "" {
		t.Fatalf("taken from a better rival: %q", id)
	}
	g := setup([]string{"Panorama", "Blue"}, nil)
	g.s.KeepID(KindArtist, rivalID, srctag.Tag{Source: "spotify"}, "search", "sp1")
	if id, _ := g.r.SpotifyArtist(ctx, artistID); id != "" {
		t.Fatalf("shared id taken: %q", id)
	}
	// A link added later wins over the kept searched id.
	g = setup(nil, nil)
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "spotify"}, "search", "oldOOOOOOOOOOOOOOOOOOO")
	g.artist("Lucy", "https://open.spotify.com/artist/newNNNNNNNNNNNNNNNNNNN")
	g.f.set("/spotify/v1/artists/newNNNNNNNNNNNNNNNNNNN", map[string]any{"id": "newNNNNNNNNNNNNNNNNNNN", "name": "Lucy"})
	if id, _ := g.r.SpotifyArtist(ctx, artistID); id != "newNNNNNNNNNNNNNNNNNNN" {
		t.Fatalf("kept search id won over the link: %q", id)
	}
}

// A kept Apple id loses to a different one MusicBrainz links.
func TestAppleKeptLosesToLink(t *testing.T) {
	g := newRig(t)
	keepIn(g)
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "applemusic"}, "us", "111")
	g.artist("Lauryn Hill", "https://music.apple.com/us/artist/x/222")
	g.f.set("/v1/catalog/us/artists/222", map[string]any{"data": []any{map[string]any{"id": "222", "attributes": map[string]any{"name": "Lauryn Hill"}}}})
	if _, id, err := g.r.AppleArtist(context.Background(), artistID); err != nil || id != "222" {
		t.Fatalf("kept id won over the link: %q %v", id, err)
	}
	if _, ext, _, _ := g.s.KeptID(KindArtist, artistID, "applemusic"); ext != "222" {
		t.Fatalf("link not kept: %q", ext)
	}
}

// EvidenceOf weighs any source's albums the same way.
func TestEvidenceOfEachSource(t *testing.T) {
	g := newRig(t)
	g.artist("Lucy")
	g.r.MBTitles = func(context.Context, string) []string { return []string{"Panorama", "Blue"} }
	g.f.set("/spotify-auth", map[string]any{"access_token": "t", "expires_in": 3600})
	g.f.set("/spotify/v1/artists/sp1/albums", map[string]any{"items": []any{map[string]any{"name": "Panorama"}, map[string]any{"name": "Blue - Single"}}})
	g.f.set("/deezer/artist/1/albums", map[string]any{"data": []any{map[string]any{"title": "Panorama"}, map[string]any{"title": "Blue"}}})
	g.f.set("/v1/catalog/us/artists/111/albums", map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"name": "Panorama"}},
		map[string]any{"attributes": map[string]any{"name": "Elsewhere"}}}})
	ctx := context.Background()
	for _, c := range []struct {
		src, id string
		ok      bool
	}{{"spotify", "sp1", true}, {"deezer", "1", true}, {"applemusic", "111", false}} {
		ev, err := g.r.EvidenceOf(ctx, c.src, "us", c.id, artistID, nil)
		if err != nil || ev.OK != c.ok {
			t.Fatalf("%s: %+v %v", c.src, ev, err)
		}
	}
}

// SetVia keeps how a match was found; a new id clears it, the same id keeps
// it; UnverifiedRows lists the matches not yet classified.
func TestFoundByKept(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "deezer"}, "search", "1")
	g.s.KeepID(KindArtist, rivalID, srctag.Tag{Source: "deezer"}, "", "")
	if rows, _ := g.s.UnverifiedRows(ctx, "deezer"); len(rows) != 1 || rows[0].MBID != artistID {
		t.Fatalf("unverified %v", rows)
	}
	g.s.SetVia(KindArtist, artistID, "deezer", ViaBarcode)
	if v := g.s.FoundBy(KindArtist, artistID, "deezer"); v != ViaBarcode {
		t.Fatalf("found by %q", v)
	}
	if rows, _ := g.s.UnverifiedRows(ctx, "deezer"); len(rows) != 0 {
		t.Fatalf("classified row listed %v", rows)
	}
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "deezer"}, "search", "1")
	if v := g.s.FoundBy(KindArtist, artistID, "deezer"); v != ViaBarcode {
		t.Fatalf("same id lost its via: %q", v)
	}
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "deezer"}, "search", "2")
	if v := g.s.FoundBy(KindArtist, artistID, "deezer"); v != "" {
		t.Fatalf("new id kept the old via: %q", v)
	}
}

// UnverifiedMisses lists a source's misses not yet checked.
func TestUnverifiedMisses(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "deezer"}, "", "")
	g.s.KeepID(KindArtist, rivalID, srctag.Tag{Source: "deezer"}, "search", "1")
	if rows, _ := g.s.UnverifiedMisses(ctx, "deezer"); len(rows) != 1 || rows[0].MBID != artistID {
		t.Fatalf("misses %v", rows)
	}
	g.s.SetVia(KindArtist, artistID, "deezer", "checked")
	if rows, _ := g.s.UnverifiedMisses(ctx, "deezer"); len(rows) != 0 {
		t.Fatalf("checked miss listed %v", rows)
	}
}

// ExactHolders answers the artists holding a source id found exactly (a
// MusicBrainz link, a barcode or Wikidata), never by name.
func TestExactHolders(t *testing.T) {
	g := newRig(t)
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "applemusic"}, "us", "161441806")
	g.s.SetVia(KindArtist, artistID, "applemusic", ViaBarcode)
	g.s.KeepID(KindArtist, rivalID, srctag.Tag{Source: "applemusic"}, "us", "161441806")
	g.s.SetVia(KindArtist, rivalID, "applemusic", ViaName)
	if got := g.s.ExactHolders("applemusic", "161441806"); len(got) != 1 || got[0] != artistID {
		t.Fatalf("holders %v", got)
	}
}
