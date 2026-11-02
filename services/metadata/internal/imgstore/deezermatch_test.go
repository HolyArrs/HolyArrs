package imgstore

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// mbStore is a fake slim copy: stored answers by kind:mbid.
type mbStore map[string]any

func (m mbStore) get(_ context.Context, kind, mbid string) ([]byte, string, bool) {
	v, ok := m[kind+":"+mbid]
	if !ok {
		return nil, "", false
	}
	b, _ := json.Marshal(map[string]any{"source_data": v})
	return b, "2026-10-25", true
}

type keptRow struct {
	t          srctag.Tag
	store, ext string
}

// matchRig is a resolver with a fake slim copy and an in-memory kept table.
func matchRig(t *testing.T) (*rig, mbStore, map[string]keptRow) {
	g := newRig(t)
	mb := mbStore{}
	kept := map[string]keptRow{}
	g.r.MB = mb.get
	g.r.Keep = func(kind, mbid string, tg srctag.Tag, store, ext string) {
		kept[kind+":"+mbid+":"+tg.Source] = keptRow{tg, store, ext}
	}
	g.r.Kept = func(kind, mbid, source string) (string, string, time.Time, bool) {
		k, ok := kept[kind+":"+mbid+":"+source]
		return k.store, k.ext, time.Now(), ok
	}
	return g, mb, kept
}

func rel(u string) map[string]any { return map[string]any{"url": map[string]any{"resource": u}} }

func group(title, date string, rels []any, barcodes []string, lengths ...int64) map[string]any {
	var rs []any
	for _, b := range barcodes {
		rs = append(rs, map[string]any{"barcode": b})
	}
	var tr []any
	for _, l := range lengths {
		tr = append(tr, map[string]any{"length": l})
	}
	return map[string]any{"title": title, "first-release-date": date, "relations": rels, "releases": rs,
		"artist-credit": []any{map[string]any{"artist": map[string]any{"id": artistID, "name": "Low"}}},
		"media":         []any{map[string]any{"tracks": tr}}}
}

func TestDeezerMatchArtistByLinkThenStrictSearch(t *testing.T) {
	ctx := context.Background()
	g, mb, kept := matchRig(t)
	mb["artist:"+artistID] = map[string]any{"name": "Low", "relations": []any{rel("https://www.deezer.com/artist/1234")}}
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	id, how, err := g.r.DeezerMatch(ctx, KindArtist, artistID)
	if err != nil || id != "1234" || how != MatchLink {
		t.Fatalf("link first: %q %q %v", id, how, err)
	}
	if k := kept[KindArtist+":"+artistID+":deezer"]; k.t.Source != "deezer" || k.t.Ref != "1234" || k.store != MatchLink {
		t.Fatalf("kept with its source tag: %+v", k)
	}

	// No link: exact name with a clear fan lead.
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = map[string]any{"name": "Low"}
	g.f.set("/deezer/search/artist?Low", map[string]any{"data": []any{
		map[string]any{"id": 1, "name": "Low", "nb_fan": 50000}, map[string]any{"id": 2, "name": "Low", "nb_fan": 300},
		map[string]any{"id": 3, "name": "Lower", "nb_fan": 900000}}})
	if id, how, _ := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "1" || how != MatchSearch {
		t.Fatalf("clear fan lead: %q %q", id, how)
	}

	// Two same-name artists without a clear lead: unsure is unmatched.
	g, mb, kept = matchRig(t)
	mb["artist:"+artistID] = map[string]any{"name": "Low"}
	g.f.set("/deezer/search/artist?Low", map[string]any{"data": []any{
		map[string]any{"id": 1, "name": "Low", "nb_fan": 5000}, map[string]any{"id": 2, "name": "Low", "nb_fan": 3000}}})
	if id, _, err := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "" || err != nil {
		t.Fatalf("no clear lead, no match: %q %v", id, err)
	}
	if k, ok := kept[KindArtist+":"+artistID+":deezer"]; !ok || k.ext != "" {
		t.Fatalf("the miss is kept: %+v", k)
	}

	// Not in the slim copy: asked again later, nothing kept.
	g, _, kept = matchRig(t)
	if _, _, err := g.r.DeezerMatch(ctx, KindArtist, artistID); err != ErrNotKnown || len(kept) != 0 {
		t.Fatalf("unknown subject: %v %v", err, kept)
	}
}

func TestDeezerMatchAlbum(t *testing.T) {
	ctx := context.Background()
	artist := map[string]any{"name": "Low", "relations": []any{rel("https://www.deezer.com/artist/1234")}}

	// Barcode by the same artist.
	g, mb, _ := matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = group("Double Negative", "2018-09-14", nil, []string{"0098787123456"}, 1, 1, 1)
	g.f.set("/deezer/album/upc:098787123456", map[string]any{"id": 77, "nb_tracks": 3, "artist": map[string]any{"id": 1234, "name": "Low"}})
	if id, how, err := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "77" || how != MatchUPC || err != nil {
		t.Fatalf("barcode: %q %q %v", id, how, err)
	}

	// Barcode by another artist is refused; the checked search decides.
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = group("Double Negative", "2018-09-14", nil, []string{"98787123456"}, 100000, 100000)
	g.f.set("/deezer/album/upc:98787123456", map[string]any{"id": 66, "nb_tracks": 2, "artist": map[string]any{"id": 9, "name": "Other"}})
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{
		map[string]any{"id": 80, "title": "Double Negative", "release_date": "2014-01-01"}, // year off
		map[string]any{"id": 81, "title": "Double Negative", "release_date": "2018-09-14"}}})
	g.f.set("/deezer/album/81", map[string]any{"id": 81, "nb_tracks": 2, "duration": 202, "artist": map[string]any{"id": 1234}})
	if id, how, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "81" || how != MatchSearch {
		t.Fatalf("checked search: %q %q", id, how)
	}

	// Same title in the artist's list, but another main artist: refused.
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = group("Do It Properly", "2011", nil, nil, 100000, 100000)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{map[string]any{"id": 90, "title": "Do It Properly", "release_date": "2011-01-01"}}})
	g.f.set("/deezer/album/90", map[string]any{"id": 90, "nb_tracks": 2, "duration": 200, "artist": map[string]any{"id": 4321}})
	if id, _, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "" {
		t.Fatalf("an album the artist only appears on: %q", id)
	}

	// Length 20% off: unsure, unmatched.
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = group("Double Negative", "2018", nil, nil, 100000, 100000)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{map[string]any{"id": 81, "title": "Double Negative", "release_date": "2018-09-14"}}})
	g.f.set("/deezer/album/81", map[string]any{"id": 81, "nb_tracks": 2, "duration": 240, "artist": map[string]any{"id": 1234}})
	if id, _, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "" {
		t.Fatalf("length off: %q", id)
	}

	// A Deezer link on the release group wins, and the cover chain uses it.
	g, mb, _ = matchRig(t)
	mb["release-group:"+albumID] = group("Double Negative", "2018", []any{rel("https://www.deezer.com/en/album/55")}, nil)
	g.f.set("/deezer/album/55", map[string]any{"id": 55, "cover_xl": "https://e-cdns-images.dzcdn.net/images/cover/abc/1000x1000-000000-80-0-0.jpg"})
	g.album("Low", "Double Negative", "")
	g.r.AppleMusic = nil
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindAlbum, albumID)
	if rec.Provider != "deezer" || rec.Upgrade != "applemusic" {
		t.Fatalf("Apple off: Deezer by link, tagged for an Apple upgrade: %+v", rec)
	}
	if g.f.hit(`/deezer/search/album`) != 0 {
		t.Fatal("no name search when matched by ID")
	}

	// Apple switched off by the kill switch (key still loaded) tags too.
	g, mb, _ = matchRig(t)
	mb["release-group:"+albumID] = group("Double Negative", "2018", []any{rel("https://www.deezer.com/en/album/55")}, nil)
	g.f.set("/deezer/album/55", map[string]any{"id": 55, "cover_xl": "https://e-cdns-images.dzcdn.net/images/cover/abc/1000x1000-000000-80-0-0.jpg"})
	g.album("Low", "Double Negative", "")
	g.r.On = func(p string) bool { return p != "applemusic" }
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindAlbum, albumID); rec.Provider != "deezer" || rec.Upgrade != "applemusic" {
		t.Fatalf("Apple switched off: Deezer tagged for an Apple upgrade: %+v", rec)
	}
}

// Apple's queue past the threshold overflows to the later sources, which
// are tagged for an upgrade; a short queue keeps Apple first, untagged.
func TestAppleOverflow(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.album("Low", "Double Negative", "US")
	g.f.set("/v1/catalog/us/search?Low Double Negative", appleAlbums(g.f, "Double Negative", "Low", 1000))
	g.f.set("/caa/release-group/"+albumID, map[string]any{"images": []any{}})
	g.f.pics["/caa/release-group/"+albumID+"/front-500"] = picture(500, 500, 3)
	g.s.o.HTTP = rewrite(g.f, "")
	g.r.AppleOverflow = time.Second
	g.r.Limits.SetInterval("applemusic", time.Hour)
	if err := g.r.Limits.Allow(ctx, "applemusic"); err != nil { // takes the free slot
		t.Fatal(err)
	}
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindAlbum, albumID)
	if rec.Provider != "caa" || rec.Upgrade != "applemusic" || g.f.hit("/v1/catalog/us/search") != 0 {
		t.Fatalf("overflow past Apple, tagged: %+v", rec)
	}
	if d := rec.Recheck.Sub(rec.Fetched); d > UpgradeRecheck+time.Minute {
		t.Fatalf("upgrade on the refresh schedule soon, got %v", d)
	}

	// Queue short again: the refresh puts Apple's version in, untagged.
	g.r.Limits = NewLimits(nil)
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ = g.s.Record(ctx, KindAlbum, albumID)
	if rec.Provider != "applemusic" || rec.Upgrade != "" {
		t.Fatalf("Apple replaces the overflow picture: %+v", rec)
	}
}

// The resolver sends Deezer and Spotify through its PoolHTTP client, and
// never Apple or the Cover Art Archive. (The archive does reach the pool,
// but one level up: cmd/metadata poolForCAA swaps the shared client's
// transport for its hosts; see TestCoverArtArchiveOverPool there.)
func TestResolverPoolClientOnlyForDeezerAndSpotify(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	var pooled atomic.Int32
	base := g.f.srv.Client().Transport
	g.r.PoolHTTP = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		pooled.Add(1)
		if r.URL.Path == "/v1/catalog/us/search" || len(r.URL.Path) > 4 && r.URL.Path[:5] == "/caa/" {
			t.Errorf("Apple or the archive went through the resolver pool client: %s", r.URL.Path)
		}
		return base.RoundTrip(r)
	})}
	g.artist("Low", "https://www.deezer.com/artist/1234")
	g.f.set("/deezer/artist/1234", map[string]any{"name": "Low"})
	g.r.Keys = Keys{}
	_ = g.s.Fetch(ctx, KindArtist, artistID)
	g.album("Low", "Double Negative", "")
	_ = g.s.Fetch(ctx, KindAlbum, albumID)
	if pooled.Load() == 0 || g.f.hit("/deezer/artist/1234") == 0 {
		t.Fatal("Deezer went direct")
	}
}

func TestEditionBase(t *testing.T) {
	for _, c := range [][2]string{
		{"Abbey Road (Remastered 2009)", "abbey road"}, {"OK Computer [Deluxe Edition]", "ok computer"},
		{"Help! - Remastered 2009", "help"}, {"Abbey Road (Super Deluxe Edition) (2019 Mix)", "abbey road"},
		{"Pet Sounds (Mono & Stereo)", "pet sounds"}, {"Lonerism (10th Anniversary Expanded Edition)", "lonerism"},
		{"Unplugged (Live)", "unplugged live"}, {"Fearless (Taylor's Version)", "fearless taylors version"},
	} {
		if got := EditionBase(c[0]); got != c[1] {
			t.Errorf("%q: %q, want %q", c[0], got, c[1])
		}
	}
}

// Fix 1: a linked Deezer artist that holds few of the artist's titles
// (a duplicate) loses to the top same-name candidate with more; a strong
// link stays, and a candidate with no more titles never replaces it.
func TestDeezerMatchWeakArtistLink(t *testing.T) {
	ctx := context.Background()
	titles := []string{"OK Computer", "Kid A", "In Rainbows", "The Bends"}
	setup := func(dupe []any) (*rig, map[string]keptRow) {
		g, mb, kept := matchRig(t)
		g.r.MBTitles = func(context.Context, string) []string { return titles }
		mb["artist:"+artistID] = map[string]any{"name": "Radiohead", "relations": []any{rel("https://www.deezer.com/artist/323887691")}}
		g.f.set("/deezer/artist/323887691", map[string]any{"id": 323887691, "name": "Radiohead"})
		g.f.set("/deezer/artist/323887691/albums", map[string]any{"data": dupe})
		g.f.set("/deezer/search/artist?Radiohead", map[string]any{"data": []any{
			map[string]any{"id": 399, "name": "Radiohead", "nb_fan": 3000000}, map[string]any{"id": 323887691, "name": "Radiohead", "nb_fan": 40}}})
		g.f.set("/deezer/artist/399/albums", map[string]any{"data": []any{
			map[string]any{"id": 1, "title": "OK Computer"}, map[string]any{"id": 2, "title": "Kid A Mnesia (Deluxe)"},
			map[string]any{"id": 3, "title": "In Rainbows"}, map[string]any{"id": 4, "title": "The Bends (Remastered)"}}})
		return g, kept
	}
	// MusicBrainz's link always wins, however few titles it holds.
	g, _ := setup([]any{map[string]any{"id": 9, "title": "OK Computer"}})
	if id, how, err := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "323887691" || how != MatchLink || err != nil {
		t.Fatalf("a weak link was replaced by a searched id: %q %q %v", id, how, err)
	}
	g, _ = setup([]any{map[string]any{"id": 9, "title": "OK Computer"}, map[string]any{"id": 8, "title": "Kid A"}, map[string]any{"id": 7, "title": "In Rainbows"}})
	if id, how, _ := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "323887691" || how != MatchLink {
		t.Fatalf("a strong link stays: %q %q", id, how)
	}
	if g.f.hit("/deezer/search/artist") != 0 {
		t.Fatal("no search for a strong link")
	}
	g, _ = setup([]any{map[string]any{"id": 9, "title": "OK Computer"}, map[string]any{"id": 8, "title": "Kid A"}})
	g.f.set("/deezer/artist/399/albums", map[string]any{"data": []any{map[string]any{"id": 1, "title": "OK Computer"}}})
	if id, how, _ := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "323887691" || how != MatchLink {
		t.Fatalf("a candidate with fewer titles never replaces the link: %q %q", id, how)
	}
}

// groupEd is a release group with official editions of the given track
// counts and the chosen edition's lengths.
func groupEd(title, date string, counts []int, lengths ...int64) map[string]any {
	g := group(title, date, nil, nil, lengths...)
	var rs []any
	for _, n := range counts {
		rs = append(rs, map[string]any{"status": "Official", "track-count": n})
	}
	g["releases"] = rs
	return g
}

// Fix 2: edition words on the Deezer side, several versions, the earliest
// one dated years after the original.
func TestDeezerMatchEditionWords(t *testing.T) {
	ctx := context.Background()
	artist := map[string]any{"name": "Low", "relations": []any{rel("https://www.deezer.com/artist/1234")}}
	ab := make([]int64, 17)
	for i := range ab {
		ab[i] = 167000 // 2839 s in all
	}
	g, mb, _ := matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = groupEd("Abbey Road", "1969-09-26", []int{17}, ab...)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{
		map[string]any{"id": 92, "title": "Abbey Road (Super Deluxe Edition)", "release_date": "2019-09-27"},
		map[string]any{"id": 91, "title": "Abbey Road (Remastered 2009)", "release_date": "2009-09-09"},
		map[string]any{"id": 93, "title": "Abbey Road Live", "release_date": "1969-09-26"}}})
	g.f.set("/deezer/album/91", map[string]any{"id": 91, "nb_tracks": 17, "duration": 2840, "artist": map[string]any{"id": 1234}})
	g.f.set("/deezer/album/92", map[string]any{"id": 92, "nb_tracks": 40, "duration": 9000, "artist": map[string]any{"id": 1234}})
	if id, how, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "91" || how != MatchSearch {
		t.Fatalf("earliest edition-word version: %q %q", id, how)
	}

	// Another tail ("(Live)") keeps the strict year rule.
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = groupEd("Unplugged", "1994", []int{2}, 100000, 100000)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{
		map[string]any{"id": 71, "title": "Unplugged (Live)", "release_date": "2004-01-01"},
		map[string]any{"id": 72, "title": "Unplugged (Live)", "release_date": "2010-01-01"}}})
	g.f.set("/deezer/album/71", map[string]any{"id": 71, "nb_tracks": 2, "duration": 200, "artist": map[string]any{"id": 1234}})
	if id, _, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "" {
		t.Fatalf("non-edition tail, years off, several versions: %q", id)
	}
}

// Fix 3: the chosen MusicBrainz edition is a big one; the Deezer album
// matches the smallest official edition. A different count with the same
// base and no edition support stays unmatched.
func TestDeezerMatchSmallestEdition(t *testing.T) {
	ctx := context.Background()
	artist := map[string]any{"name": "Low", "relations": []any{rel("https://www.deezer.com/artist/1234")}}
	big := make([]int64, 26)
	for i := range big {
		big[i] = 200000
	}
	g, mb, _ := matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = groupEd("Lonerism", "2012-10-05", []int{26, 12}, big...)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{map[string]any{"id": 61, "title": "Lonerism", "release_date": "2012-10-05"}}})
	g.f.set("/deezer/album/61", map[string]any{"id": 61, "nb_tracks": 12, "duration": 3100, "artist": map[string]any{"id": 1234}})
	if id, how, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "61" || how != MatchSearch {
		t.Fatalf("smallest official edition: %q %q", id, how)
	}

	// Far fewer tracks than any edition (a single or an EP of the name).
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = groupEd("Lonerism", "2012-10-05", []int{26, 20}, big...)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{map[string]any{"id": 61, "title": "Lonerism", "release_date": "2012-10-05"}}})
	g.f.set("/deezer/album/61", map[string]any{"id": 61, "nb_tracks": 3, "duration": 700, "artist": map[string]any{"id": 1234}})
	if id, _, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "" {
		t.Fatalf("a 3-track release of the name: %q", id)
	}

	// A bonus track: one more track and a longer length is the same album;
	// the same count with another length is not.
	ten := []int64{200000, 200000, 200000, 200000, 200000, 200000, 200000, 200000, 200000, 200000}
	for _, c := range []struct {
		tracks int
		secs   int
		want   string
	}{{11, 2240, "62"}, {10, 2400, ""}, {11, 1700, ""}} {
		g, mb, _ = matchRig(t)
		mb["artist:"+artistID] = artist
		g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
		mb["release-group:"+albumID] = groupEd("The Fame", "2008-08-19", []int{10}, ten...)
		g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{map[string]any{"id": 62, "title": "The Fame", "release_date": "2008-10-28"}}})
		g.f.set("/deezer/album/62", map[string]any{"id": 62, "nb_tracks": c.tracks, "duration": c.secs, "artist": map[string]any{"id": 1234}})
		if id, _, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != c.want {
			t.Fatalf("%d tracks, %d s: %q, want %q", c.tracks, c.secs, id, c.want)
		}
	}

	// A self-titled album: an expanded reissue years later may be another
	// album of the same name, so it stays unmatched.
	g, mb, _ = matchRig(t)
	mb["artist:"+artistID] = artist
	g.f.set("/deezer/artist/1234", map[string]any{"id": 1234, "name": "Low"})
	mb["release-group:"+albumID] = groupEd("Low", "1976-02-10", []int{10}, ten...)
	g.f.set("/deezer/artist/1234/albums", map[string]any{"data": []any{
		map[string]any{"id": 63, "title": "Low", "release_date": "1970-06-01"},
		map[string]any{"id": 64, "title": "Low (Expanded Edition)", "release_date": "2012-02-21"}}})
	g.f.set("/deezer/album/64", map[string]any{"id": 64, "nb_tracks": 11, "duration": 2300, "artist": map[string]any{"id": 1234}})
	if id, _, _ := g.r.DeezerMatch(ctx, KindAlbum, albumID); id != "" {
		t.Fatalf("self-titled reissue years later: %q", id)
	}
}

// H4: a search winner by fan lead alone is not taken: it must hold enough
// of the artist's album titles and not belong to another artist already.
func TestDeezerSearchWinnerNeedsTitlesAndUnsharedID(t *testing.T) {
	ctx := context.Background()
	setup := func(titles []string) *rig {
		g, mb, _ := matchRig(t)
		g.r.MBTitles = func(context.Context, string) []string { return titles }
		mb["artist:"+artistID] = map[string]any{"name": "Low"}
		g.f.set("/deezer/search/artist?Low", map[string]any{"data": []any{
			map[string]any{"id": 1, "name": "Low", "nb_fan": 500000}, map[string]any{"id": 2, "name": "Low", "nb_fan": 30}}})
		g.f.set("/deezer/artist/1/albums", map[string]any{"data": []any{
			map[string]any{"id": 11, "title": "Things We Lost in the Fire"}, map[string]any{"id": 12, "title": "Double Negative"},
			map[string]any{"id": 13, "title": "C'mon"}}})
		return g
	}
	// An obscure homonym whose albums the famous one does not hold.
	g := setup([]string{"Basement Tapes", "Demo One", "Live at Home"})
	if id, _, err := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "" || err != nil {
		t.Fatalf("homonym taken on fans alone: %q %v", id, err)
	}
	// The real artist: titles in common.
	g = setup([]string{"Things We Lost in the Fire", "Double Negative", "C'mon", "HEY WHAT"})
	if id, how, err := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "1" || how != MatchSearch || err != nil {
		t.Fatalf("real artist: %q %q %v", id, how, err)
	}
	// Titles match, but the Deezer id is already another artist's.
	g = setup([]string{"Things We Lost in the Fire", "Double Negative", "C'mon"})
	g.r.Shared = func(kind, source, ext, except string) bool {
		return source == "deezer" && ext == "1" && except == artistID
	}
	if id, _, _ := g.r.DeezerMatch(ctx, KindArtist, artistID); id != "" {
		t.Fatalf("shared id taken: %q", id)
	}
}
