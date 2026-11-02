package imgstore

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

func keepIn(g *rig) {
	g.r.Keep, g.r.Kept = g.s.KeepID, g.s.KeptID
}

func TestAlbumMatchKeptForTheDetails(t *testing.T) {
	g := newRig(t)
	keepIn(g)
	g.album("Frank Ocean", "Blonde", "")
	hit := appleAlbums(g.f, "Blonde", "Frank Ocean", 3000)
	hit["results"].(map[string]any)["albums"].(map[string]any)["data"].([]any)[0].(map[string]any)["id"] = "1146195596"
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", hit)
	ctx := context.Background()
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	searches := g.f.hit("/v1/catalog/us/search")
	st, id, err := g.r.AppleAlbumID(ctx, albumID)
	if err != nil || st != "us" || id != "1146195596" {
		t.Fatalf("%q %q %v", st, id, err)
	}
	if g.f.hit("/v1/catalog/us/search") != searches {
		t.Fatal("the details searched again instead of reusing the cover's match")
	}
}

func TestAlbumMissKeptOnlyWhenAppleAnswered(t *testing.T) {
	g := newRig(t)
	keepIn(g)
	g.album("Low", "Nothing", "")
	ctx := context.Background()
	g.f.set("/v1/catalog/us/search?Low Nothing", 503)
	if _, _, err := g.r.AppleAlbumID(ctx, albumID); err == nil {
		t.Fatal("a 503 answered no album")
	}
	if _, _, _, ok := g.s.KeptID(KindAlbum, albumID, "applemusic"); ok {
		t.Fatal("a failure was kept")
	}
	g.f.set("/v1/catalog/us/search?Low Nothing", map[string]any{"results": map[string]any{}})
	if st, id, err := g.r.AppleAlbumID(ctx, albumID); err != nil || st != "" || id != "" {
		t.Fatalf("%q %q %v", st, id, err)
	}
	if _, ext, _, ok := g.s.KeptID(KindAlbum, albumID, "applemusic"); !ok || ext != "" {
		t.Fatal("the miss was not kept")
	}
	n := g.f.hit("/v1/catalog/us/search")
	if _, _, err := g.r.AppleAlbumID(ctx, albumID); err != nil || g.f.hit("/v1/catalog/us/search") != n {
		t.Fatal("a kept miss searched again")
	}
}

func TestSpotifyArtistKept(t *testing.T) {
	g := newRig(t)
	keepIn(g)
	g.artist("Frank Ocean")
	g.f.set("/spotify/v1/search?Frank Ocean", map[string]any{"artists": map[string]any{"items": []any{
		map[string]any{"id": "other", "name": "Frank Oceanic"},
		map[string]any{"id": "2h93pZq0e7k5yf4dywlkpM", "name": "Frank Ocean"}}}})
	ctx := context.Background()
	id, err := g.r.SpotifyArtist(ctx, artistID)
	if err != nil || id != "2h93pZq0e7k5yf4dywlkpM" {
		t.Fatalf("%q %v", id, err)
	}
	if how, ext, _, ok := g.s.KeptID(KindArtist, artistID, "spotify"); !ok || ext != id || how != "search" {
		t.Fatalf("kept %q %q", how, ext)
	}
	// A name search's match is never answered as a link.
	if _, ok := g.s.LinkedSpotify(artistID); ok {
		t.Fatal("a search match answered as a link")
	}
	if _, ok := g.s.LinkedArtist(id); ok {
		t.Fatal("a search match answered as a link")
	}
	// From the artist's MusicBrainz link: answered both ways.
	g2 := newRig(t)
	keepIn(g2)
	g2.artist("Frank Ocean", "https://open.spotify.com/artist/2h93pZq0e7k5yf4dywlkpM")
	if id, err := g2.r.SpotifyArtist(ctx, artistID); err != nil || id != "2h93pZq0e7k5yf4dywlkpM" {
		t.Fatalf("%q %v", id, err)
	}
	if sp, ok := g2.s.LinkedSpotify(artistID); !ok || sp != "2h93pZq0e7k5yf4dywlkpM" {
		t.Fatal("link match not answered")
	}
	if mb, ok := g2.s.LinkedArtist("2h93pZq0e7k5yf4dywlkpM"); !ok || mb != artistID {
		t.Fatal("link match not answered from the Spotify side")
	}
}

// Two artists of one name matched to one Apple artist by name: neither gets
// that artist's similar artists and pictures, unless MusicBrainz links it.
func TestAppleArtistSharedByNameIsDistrusted(t *testing.T) {
	const other = "b84b1b7f-71a5-4011-9441-d0b5e4122712"
	g := newRig(t)
	keepIn(g)
	g.r.Shared = g.s.SharedID
	g.artist("Utopia")
	g.info[KindArtist+":"+other] = Subject{Known: true, Name: "Utopia"}
	g.f.set("/v1/catalog/us/search?Utopia", map[string]any{"results": map[string]any{"artists": map[string]any{"data": []any{
		map[string]any{"id": "111", "attributes": map[string]any{"name": "Utopia"}}}}}})
	ctx := context.Background()
	if _, id, err := g.r.AppleArtist(ctx, artistID); err != nil || id != "111" {
		t.Fatalf("first match %q %v", id, err)
	}
	if _, id, err := g.r.AppleArtist(ctx, other); err != nil || id != "" {
		t.Fatalf("the second artist of the name was given %q %v", id, err)
	}
	// The first is now shared too, so it is no longer trusted either.
	g.r.memo = nil
	if _, id, err := g.r.AppleArtist(ctx, artistID); err != nil || id != "" {
		t.Fatalf("a shared match was trusted: %q %v", id, err)
	}
	// MusicBrainz's own Apple link keeps the artist its match.
	g.f.set("/v1/catalog/us/artists/111", map[string]any{"data": []any{map[string]any{"id": "111", "attributes": map[string]any{"name": "Utopia"}}}})
	g.info[KindArtist+":"+artistID] = Subject{Known: true, Name: "Utopia", Links: []string{"https://music.apple.com/us/artist/utopia/111"}}
	g.r.memo = nil
	if _, id, err := g.r.AppleArtist(ctx, artistID); err != nil || id != "111" {
		t.Fatalf("a linked match was dropped: %q %v", id, err)
	}
}

// M5: a name-only Apple match with same-name artists in the slim copy is
// taken only on evidence: the Apple artist's albums hold enough of the
// artist's release-group titles and clearly more than any rival's. A rival
// that fits better, or no overlap at all, is a kept miss; a linked match
// and a name nobody else carries are taken as before.
func TestAppleNameOnlyMatchNeedsAlbumEvidence(t *testing.T) {
	const rival = "d04b1b7f-71a5-4011-9441-d0b5e4122719"
	ctx := context.Background()
	run := func(t *testing.T, mine, theirs, apple []string, rivals bool) string {
		g := newRig(t)
		keepIn(g)
		g.r.Shared = g.s.SharedID
		g.r.MBTitles = func(_ context.Context, id string) []string {
			if id == rival {
				return theirs
			}
			return mine
		}
		g.r.TitleArtists = func(context.Context, string) int { return 1 }
		g.r.Homonyms = func(_ context.Context, name, except string) []string {
			if rivals && name == "Utopia" && except == artistID {
				return []string{rival}
			}
			return nil
		}
		g.artist("Utopia")
		g.f.set("/v1/catalog/us/search?Utopia", map[string]any{"results": map[string]any{"artists": map[string]any{"data": []any{
			map[string]any{"id": "111", "attributes": map[string]any{"name": "Utopia"}}}}}})
		var data []any
		for _, a := range apple {
			data = append(data, map[string]any{"id": "x", "attributes": map[string]any{"name": a}})
		}
		g.f.set("/v1/catalog/us/artists/111/albums", map[string]any{"data": data})
		_, id, err := g.r.AppleArtist(ctx, artistID)
		if err != nil {
			t.Fatal(err)
		}
		if _, ext, _, ok := g.s.KeptID(KindArtist, artistID, "applemusic"); !ok || ext != id {
			t.Fatalf("kept %q %v, answered %q", ext, ok, id)
		}
		return id
	}
	mine := []string{"Ra", "Oops! Wrong Planet", "Adventures in Utopia", "Swing to the Right"}
	t.Run("albums fit", func(t *testing.T) {
		if id := run(t, mine, []string{"Utopia Dub"}, []string{"Ra", "Oops! Wrong Planet (Remastered)", "Adventures in Utopia", "Live"}, true); id != "111" {
			t.Fatalf("a fitting match was refused: %q", id)
		}
	})
	t.Run("rival fits better", func(t *testing.T) {
		if id := run(t, []string{"Ra"}, []string{"Utopia Dub", "Deep", "Live at Home"}, []string{"Ra", "Utopia Dub", "Deep", "Live at Home"}, true); id != "" {
			t.Fatalf("taken from a better rival: %q", id)
		}
	})
	t.Run("rival as good", func(t *testing.T) {
		if id := run(t, []string{"Ra", "Deep"}, []string{"Ra", "Deep"}, []string{"Ra", "Deep"}, true); id != "" {
			t.Fatalf("taken on a tie: %q", id)
		}
	})
	t.Run("nothing in common", func(t *testing.T) {
		if id := run(t, mine, nil, []string{"Something Else"}, true); id != "" {
			t.Fatalf("taken with no overlap: %q", id)
		}
	})
	t.Run("no titles known", func(t *testing.T) {
		if id := run(t, nil, nil, []string{"Ra"}, true); id != "" {
			t.Fatalf("taken without evidence: %q", id)
		}
	})
	t.Run("apple tags, double titles and hyphens", func(t *testing.T) {
		if id := run(t, []string{"Les 150 Passions meurtrières", "Heavy-Metal Jazz", "Dust", "Hard Attack"}, nil,
			[]string{"Les 150 Passions Meurtrieres - EP", "HeavyMetal Jazz", "Hard Attack/Dust"}, true); id != "111" {
			t.Fatalf("the same titles written apart were refused: %q", id)
		}
	})
	t.Run("a self-titled album is no evidence", func(t *testing.T) {
		if id := run(t, []string{"Utopia"}, nil, []string{"Utopia", "Other"}, true); id != "" {
			t.Fatalf("taken on the artist's own name: %q", id)
		}
	})
	t.Run("two titles and no rival", func(t *testing.T) {
		if id := run(t, []string{"Forever War", "Cornered", "No Surrender", "The Meaning of Pain"}, []string{"Other"}, []string{"Forever War", "Cornered - EP"}, true); id != "111" {
			t.Fatalf("two distinct titles with no rival refused: %q", id)
		}
	})
	t.Run("feat, from and with decorations", func(t *testing.T) {
		if id := run(t, []string{"Bacio di Giuda", "Semplicemente", "Rose nere"}, nil,
			[]string{"Bacio di Giuda (feat. Villabanks)", "Semplicemente (From the Netflix Series \"Riv4li\")", "Rose Nere feat. Mida - Single"}, true); id != "111" {
			t.Fatalf("decorated titles refused: %q", id)
		}
	})
	t.Run("apple's only album in common", func(t *testing.T) {
		if id := run(t, []string{"Sukierae", "Summer Noon", "Diamond Light"}, []string{"Other"}, []string{"Sukierae"}, true); id != "111" {
			t.Fatalf("the whole Apple catalog in common refused: %q", id)
		}
	})
	t.Run("a self-titled part of a double title is no evidence", func(t *testing.T) {
		if id := run(t, []string{"Utopia / Hell n' Back", "Ra", "Deep"}, nil, []string{"Utopia"}, true); id != "" {
			t.Fatalf("taken on the artist's own name inside a double title: %q", id)
		}
	})
	t.Run("no homonym", func(t *testing.T) {
		if id := run(t, nil, nil, nil, false); id != "111" {
			t.Fatalf("a unique name lost its match: %q", id)
		}
	})
	if !AppleNameOnly(nil, "111") || AppleNameOnly([]string{"https://music.apple.com/us/artist/utopia/111"}, "111") {
		t.Fatal("AppleNameOnly")
	}
}

// Expire clears a picture's miss and makes a stored one due, so the next
// queue job asks the sources again.
func TestExpireMakesPictureDue(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde", "Frank Ocean", 1000))
	ctx := context.Background()
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if !g.s.Fresh(ctx, KindAlbum, albumID) {
		t.Fatal("not fresh after fetch")
	}
	g.s.Expire(ctx, KindAlbum, albumID)
	if g.s.Fresh(ctx, KindAlbum, albumID) {
		t.Fatal("still fresh")
	}
	if _, err := g.s.db.Exec(`INSERT INTO misses(kind,mbid,until) VALUES(?,?,?)`, KindArtist, artistID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	g.s.Expire(ctx, KindArtist, artistID)
	if g.s.HasMiss(ctx, KindArtist, artistID) {
		t.Fatal("miss kept")
	}
}

func TestDropFromOnlyThatProvider(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde", "Frank Ocean", 1000))
	ctx := context.Background()
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.s.DropFrom(ctx, KindAlbum, albumID, "deezer"); ok {
		t.Fatal("dropped another provider's picture")
	}
	if rec, ok := g.s.DropFrom(ctx, KindAlbum, albumID, "applemusic"); !ok || rec.Provider != "applemusic" {
		t.Fatal("not dropped")
	}
	if _, ok := g.s.Record(ctx, KindAlbum, albumID); ok {
		t.Fatal("still stored")
	}
}

func TestEvidenceKeysDropDecorations(t *testing.T) {
	for in, want := range map[string]string{
		"Bacio di Giuda (feat. Villabanks)":       "baciodigiuda",
		"Rose Nere feat. Mida - Single":           "rosenere",
		"Rose Nere ft Mida":                       "rosenere",
		"Semplicemente (From the Netflix Series)": "semplicemente",
		"Stay [with Justin Bieber]":               "stay",
		"Blue Note Ridge - Remastered 2011":       "bluenoteridge",
		"Featuring Nobody":                        "featuringnobody",
	} {
		if k := evidenceKeys(in); !slices.Contains(k, want) {
			t.Errorf("%q: keys %v, want %q among them", in, k, want)
		}
	}
}

// AppleMissesBetween lists the Apple artist misses kept in a time window;
// ForgetID drops one so the next lookup asks again.
func TestAppleMissesBetweenAndForget(t *testing.T) {
	g := newRig(t)
	t0 := time.Now()
	g.s.KeepID(KindArtist, artistID, srctag.Tag{Source: "applemusic"}, "", "")
	g.s.KeepID(KindArtist, albumID, srctag.Tag{Source: "applemusic"}, "us", "1")
	ids := g.s.AppleMissesBetween(context.Background(), t0.Add(-time.Minute), t0.Add(time.Minute))
	if len(ids) != 1 || ids[0] != artistID {
		t.Fatalf("misses %v", ids)
	}
	if got := g.s.AppleMissesBetween(context.Background(), t0.Add(time.Minute), t0.Add(2*time.Minute)); len(got) != 0 {
		t.Fatalf("outside the window %v", got)
	}
	g.s.ForgetID(KindArtist, artistID, "applemusic")
	if _, _, _, ok := g.s.KeptID(KindArtist, artistID, "applemusic"); ok {
		t.Fatal("still kept")
	}
}

// One title in common is evidence only when it is rare: at least six
// letters and held by at most rareTitleArtists artists in the slim copy.
func TestSingleSharedTitleMustBeRare(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, mine string, holders int) bool {
		g := newRig(t)
		g.artist("Roxana")
		g.r.MBTitles = func(context.Context, string) []string { return []string{mine} }
		g.r.TitleArtists = func(context.Context, string) int { return holders }
		g.f.set("/v1/catalog/us/artists/111/albums", map[string]any{"data": []any{
			map[string]any{"attributes": map[string]any{"name": mine}}, map[string]any{"attributes": map[string]any{"name": "Something Else"}}}})
		ev, err := g.r.EvidenceOf(ctx, "applemusic", "us", "111", artistID, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ev.OK
	}
	if !run(t, "Sagrada Familia", 1) {
		t.Fatal("a rare title was refused")
	}
	if run(t, "What It Do", 5) {
		t.Fatal("a title five artists hold was taken")
	}
	if run(t, "Fall", 1) {
		t.Fatal("a short title was taken")
	}
}
