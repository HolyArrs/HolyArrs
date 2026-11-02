package musicdata

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

const (
	artistA = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	artistB = "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d"
	albumA  = "6e335887-60ba-38f0-95af-fae7774336bf"
	albumB  = "1e335887-60ba-38f0-95af-fae7774336bf"
)

// fake answers by path (and the "ids", "term" or "q" query when set): a
// JSON body, or a status code as an int; anything else is a 404.
type fake struct {
	srv  *httptest.Server
	mu   sync.Mutex
	api  map[string]any
	hits map[string]int
}

func newFake(t *testing.T) *fake {
	f := &fake{api: map[string]any{}, hits: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		k := r.URL.Path
		for _, n := range []string{"ids", "term", "q"} {
			if v := q.Get(n); v != "" {
				k += "?" + v
			}
		}
		f.mu.Lock()
		f.hits[k]++
		v, ok := f.api[k]
		f.mu.Unlock()
		switch v := v.(type) {
		case int:
			w.WriteHeader(v)
		case nil:
			if !ok {
				w.WriteHeader(404)
			}
		default:
			_ = json.NewEncoder(w).Encode(v)
		}
	}))
	t.Cleanup(f.srv.Close)
	f.set("/spotify-auth", map[string]any{"access_token": "tok", "expires_in": 3600})
	return f
}

func (f *fake) set(k string, v any) {
	f.mu.Lock()
	f.api[k] = v
	f.mu.Unlock()
}

func (f *fake) hit(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[k]
}

type rig struct {
	f      *fake
	s      *Service
	info   map[string]imgstore.Subject
	stored map[string]string
}

func newRig(t *testing.T) *rig {
	f := newFake(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	g := &rig{f: f, info: map[string]imgstore.Subject{}, stored: map[string]string{}}
	res := &imgstore.Resolver{HTTP: f.srv.Client(), AppleMusic: &imgstore.AppleMusic{TeamID: "T", KeyID: "K", Key: key, Base: f.srv.URL},
		Deezer: f.srv.URL + "/deezer", SpotifyAPI: f.srv.URL + "/spotify", SpotifyAuth: f.srv.URL + "/spotify-auth",
		Keys: imgstore.Keys{SpotifyID: "id", SpotifySecret: "secret"}, Limits: imgstore.NewLimits(nil),
		Info: func(_ context.Context, kind, mbid string) imgstore.Subject { return g.info[kind+":"+mbid] }}
	var err error
	g.s, err = Open(filepath.Join(t.TempDir(), "d.sqlite"), Options{Sources: res, Stored: func(_ context.Context, kind, mbid string) ([]byte, bool) {
		b, ok := g.stored[kind+":"+mbid]
		return []byte(b), ok
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.s.Close() })
	return g
}

func (g *rig) artist(mbid, name string, links ...string) {
	g.info[imgstore.KindArtist+":"+mbid] = imgstore.Subject{Known: true, Name: name, Links: links}
}

func (g *rig) album(mbid, name, title string) {
	g.info[imgstore.KindAlbum+":"+mbid] = imgstore.Subject{Known: true, Name: name, Title: title}
}

func spotifyTracks(names ...string) map[string]any {
	var ts []any
	for _, n := range names {
		ts = append(ts, map[string]any{"name": n, "duration_ms": 200000, "explicit": true, "album": map[string]any{"name": "Album",
			"images": []any{map[string]any{"url": "https://i.test/640", "width": 640}, map[string]any{"url": "https://i.test/300", "width": 300}, map[string]any{"url": "https://i.test/64", "width": 64}}}})
	}
	return map[string]any{"tracks": ts}
}

func TestTopSongsOrder(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.artist(artistA, "Frank Ocean", "https://open.spotify.com/artist/sAAAAAAAAAAAAAAAAAAAAA", "https://music.apple.com/us/artist/111")
	g.f.set("/spotify/v1/artists/sAAAAAAAAAAAAAAAAAAAAA/top-tracks", spotifyTracks("Nights", "Ivy"))
	g.f.set("/v1/catalog/us/artists/111", map[string]any{"data": []any{map[string]any{"id": "111", "attributes": map[string]any{"name": "Frank Ocean"}}}})
	g.f.set("/v1/catalog/us/artists/111/view/top-songs", map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"name": "Apple Song", "durationInMillis": 1000, "albumName": "A", "artwork": map[string]any{"url": "https://a.test/{w}x{h}bb.{f}"}}}}})
	top, err := g.s.TopSongs(ctx, artistA)
	if err != nil || top.Source != "spotify" || len(top.Songs) != 2 || top.Songs[0].Title != "Nights" || top.Songs[0].LengthMS != 200000 || !top.Songs[0].Explicit || top.Songs[0].Cover != "https://i.test/300" {
		t.Fatalf("spotify first: %+v %v", top, err)
	}
	if g.f.hit("/v1/catalog/us/artists/111/view/top-songs") != 0 {
		t.Fatal("Apple asked although Spotify answered")
	}

	// Spotify fails: Apple answers, kept only an hour.
	g2 := newRig(t)
	g2.artist(artistA, "Frank Ocean", "https://open.spotify.com/artist/sAAAAAAAAAAAAAAAAAAAAA", "https://music.apple.com/us/artist/111")
	g2.f.set("/spotify/v1/artists/sAAAAAAAAAAAAAAAAAAAAA/top-tracks", 500)
	g2.f.set("/v1/catalog/us/artists/111", map[string]any{"data": []any{map[string]any{"id": "111", "attributes": map[string]any{"name": "Frank Ocean"}}}})
	g2.f.set("/v1/catalog/us/artists/111/view/top-songs", map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"name": "Apple Song", "durationInMillis": 1000, "contentRating": "explicit", "albumName": "A", "artwork": map[string]any{"url": "https://a.test/{w}x{h}bb.{f}"}}}}})
	top, err = g2.s.TopSongs(ctx, artistA)
	if err != nil || top.Source != "applemusic" || top.Songs[0].Cover != "https://a.test/250x250bb.jpg" || !top.Songs[0].Explicit {
		t.Fatalf("apple after a failure: %+v %v", top, err)
	}
	var until int64
	_ = g2.s.db.QueryRow(`SELECT until FROM answers WHERE k = ?`, "top:"+artistA).Scan(&until)
	if time.Until(time.Unix(until, 0)) > 2*time.Hour {
		t.Fatal("an answer after a failure was kept a week")
	}

	// Spotify and Apple have none: Deezer answers.
	g3 := newRig(t)
	g3.artist(artistA, "Frank Ocean", "https://www.deezer.com/artist/42")
	g3.f.set("/spotify/v1/search?Frank Ocean", map[string]any{"artists": map[string]any{"items": []any{}}})
	g3.f.set("/v1/catalog/us/search?Frank Ocean", map[string]any{"results": map[string]any{}})
	g3.f.set("/deezer/artist/42", map[string]any{"id": 42, "name": "Frank Ocean"})
	g3.f.set("/deezer/artist/42/top", map[string]any{"data": []any{map[string]any{"title": "Lost", "duration": 234, "album": map[string]any{"title": "Channel Orange", "cover_medium": "https://d.test/250"}}}})
	top, err = g3.s.TopSongs(ctx, artistA)
	if err != nil || top.Source != "deezer" || top.Songs[0].LengthMS != 234000 || top.Songs[0].Album != "Channel Orange" {
		t.Fatalf("deezer last: %+v %v", top, err)
	}

	// None anywhere: kept as none. A failure with no later answer: ask again.
	g4 := newRig(t)
	g4.artist(artistA, "Nobody")
	g4.f.set("/spotify/v1/search?Nobody", map[string]any{"artists": map[string]any{"items": []any{}}})
	g4.f.set("/v1/catalog/us/search?Nobody", 503)
	g4.f.set("/deezer/search/artist?Nobody", map[string]any{"data": []any{}})
	if _, err := g4.s.TopSongs(ctx, artistA); !errors.Is(err, ErrLater) {
		t.Fatalf("a 503 with nothing later: %v", err)
	}
	g4.f.set("/v1/catalog/us/search?Nobody", map[string]any{"results": map[string]any{}})
	if _, err := g4.s.TopSongs(ctx, artistA); !errors.Is(err, ErrNone) {
		t.Fatalf("none anywhere: %v", err)
	}
	// An artist whose MusicBrainz answer is not stored: ask again later.
	if _, err := g4.s.TopSongs(ctx, artistB); !errors.Is(err, ErrLater) {
		t.Fatalf("unknown artist: %v", err)
	}
}

func TestAlbumDetailsOrder(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	// MusicBrainz: track count, barcode and single from the stored release
	// group, the label from the stored tracklist release.
	g.stored["release-group:"+albumA] = `{"source_data":{"primary-type":"Album","tracklist_release":"r1","media":[{"track-count":11}],"releases":[{"id":"r0","status":"Official","barcode":"000"},{"id":"r1","status":"Official","barcode":"199316732108"}]}}`
	g.stored["release:r1"] = `{"source_data":{"label-info":[{"label":{"name":"Aftermath"}}]}}`
	g.album(albumA, "Anderson .Paak", "Heavy Is The Crown")
	g.f.set("/v1/catalog/us/search?Anderson .Paak Heavy Is The Crown", map[string]any{"results": map[string]any{"albums": map[string]any{"data": []any{
		map[string]any{"id": "900", "attributes": map[string]any{"name": "Heavy Is the Crown", "artistName": "Anderson .Paak", "artwork": map[string]any{"url": "https://a.test/{w}x{h}.jpg", "width": 3000, "height": 3000}}}}}}})
	g.f.set("/v1/catalog/us/albums/900", map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"recordLabel": "Apple Label", "trackCount": 12, "upc": "111", "contentRating": "explicit", "isSingle": false, "isComplete": true}}}})
	// albumB: MusicBrainz has nothing stored, Apple has no match, Spotify fills.
	g.album(albumB, "Laila!", "We'll Never Be This Young Again")
	g.f.set("/v1/catalog/us/search?Laila! We'll Never Be This Young Again", map[string]any{"results": map[string]any{}})
	g.f.set("/spotify/v1/search?album:\"We'll Never Be This Young Again\" artist:\"Laila!\"", map[string]any{"albums": map[string]any{"items": []any{
		map[string]any{"id": "spB", "name": "We'll Never Be This Young Again", "release_date": "2026-10-23", "artists": []any{map[string]any{"name": "Laila!"}}}}}})
	g.f.set("/spotify/v1/albums?spB", map[string]any{"albums": []any{map[string]any{"id": "spB", "label": "Warner", "total_tracks": 13, "album_type": "album",
		"external_ids": map[string]any{"upc": "093624"}, "tracks": map[string]any{"items": []any{map[string]any{"explicit": false}, map[string]any{"explicit": true}}}}}})
	got := g.s.AlbumDetails(ctx, []string{albumA, albumB})
	a := got[albumA]
	if a.Label != "Aftermath" || a.TrackCount != 11 || a.Barcode != "199316732108" || a.Sources["label"] != "musicbrainz" || a.Sources["barcode"] != "musicbrainz" {
		t.Fatalf("musicbrainz first: %+v", a)
	}
	if a.Explicit == nil || !*a.Explicit || a.Sources["explicit"] != "applemusic" || a.Complete == nil || !*a.Complete || a.Single == nil || *a.Single || a.Sources["single"] != "musicbrainz" {
		t.Fatalf("apple for the gaps: %+v", a)
	}
	if g.f.hit("/spotify/v1/albums?spB") != 1 {
		t.Fatal("spotify read once for the second album only")
	}
	b := got[albumB]
	if b.Label != "Warner" || b.TrackCount != 13 || b.Barcode != "093624" || b.Explicit == nil || !*b.Explicit || b.Sources["label"] != "spotify" {
		t.Fatalf("spotify last: %+v", b)
	}
	// albumB had no stored MusicBrainz answer: kept only an hour.
	var until int64
	_ = g.s.db.QueryRow(`SELECT until FROM answers WHERE k = ?`, "album:"+albumB).Scan(&until)
	if time.Until(time.Unix(until, 0)) > 2*time.Hour {
		t.Fatal("an answer without MusicBrainz was kept a month")
	}
}

func TestAlbumDetailsAppleFailureFallsToSpotify(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.album(albumA, "Low", "Things")
	g.stored["release-group:"+albumA] = `{"source_data":{"primary-type":"Album","releases":[]}}`
	g.f.set("/v1/catalog/us/search?Low Things", 502)
	g.f.set("/spotify/v1/search?album:\"Things\" artist:\"Low\"", map[string]any{"albums": map[string]any{"items": []any{
		map[string]any{"id": "sp", "name": "Things", "artists": []any{map[string]any{"name": "Low"}}}}}})
	g.f.set("/spotify/v1/albums?sp", map[string]any{"albums": []any{map[string]any{"id": "sp", "label": "Sub Pop", "total_tracks": 9}}})
	a := g.s.AlbumDetails(ctx, []string{albumA})[albumA]
	if a.Label != "Sub Pop" || a.Sources["label"] != "spotify" {
		t.Fatalf("%+v", a)
	}
	var until int64
	_ = g.s.db.QueryRow(`SELECT until FROM answers WHERE k = ?`, "album:"+albumA).Scan(&until)
	if time.Until(time.Unix(until, 0)) > 2*time.Hour {
		t.Fatal("kept a month after Apple failed")
	}
	if strings.Contains(a.Barcode, "x") {
		t.Fatal("unreachable")
	}
}

func TestAppleSimilar(t *testing.T) {
	g := newRig(t)
	g.artist(artistA, "Frank Ocean", "https://music.apple.com/us/artist/111")
	g.f.set("/v1/catalog/us/artists/111", map[string]any{"data": []any{map[string]any{"id": "111", "attributes": map[string]any{"name": "Frank Ocean"}}}})
	g.f.set("/v1/catalog/us/artists/111/view/similar-artists", map[string]any{"data": []any{
		map[string]any{"id": "1", "attributes": map[string]any{"name": "SZA"}}, map[string]any{"id": "2", "attributes": map[string]any{"name": "Daniel Caesar"}}}})
	got, err := g.s.AppleSimilar(context.Background(), artistA)
	if err != nil || len(got) != 2 || got[0].Name != "SZA" || got[1].ID != "2" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestDiscographyOrder(t *testing.T) {
	ctx := context.Background()
	// MusicBrainz has release groups: the app shows those.
	g := newRig(t)
	g.artist(artistA, "Frank Ocean", "https://music.apple.com/us/artist/111")
	g.stored["artist:"+artistA] = `{"source_data":{"release-groups":[{"id":"x"}]}}`
	d, err := g.s.Discography(ctx, artistA)
	if err != nil || d.Source != "musicbrainz" || len(d.Albums) != 0 || g.f.hit("/v1/catalog/us/artists/111/albums") != 0 {
		t.Fatalf("musicbrainz first: %+v %v", d, err)
	}
	// None in MusicBrainz: Apple's albums.
	g.stored["artist:"+artistB] = `{"source_data":{"release-groups":[]}}`
	g.artist(artistB, "New One", "https://music.apple.com/us/artist/222", "https://open.spotify.com/artist/spBBBBBBBBBBBBBBBBBBBB")
	g.f.set("/v1/catalog/us/artists/222", map[string]any{"data": []any{map[string]any{"id": "222", "attributes": map[string]any{"name": "New One"}}}})
	g.f.set("/v1/catalog/us/artists/222/albums", map[string]any{"data": []any{
		map[string]any{"attributes": map[string]any{"name": "Debut", "releaseDate": "2026-10-23", "trackCount": 9, "contentRating": "explicit", "upc": "123", "artwork": map[string]any{"url": "https://a.test/{w}x{h}.jpg"}}},
		map[string]any{"attributes": map[string]any{"name": "Hit - Single", "isSingle": true}}}})
	d, err = g.s.Discography(ctx, artistB)
	if err != nil || d.Source != "applemusic" || len(d.Albums) != 2 || d.Albums[0].Barcode != "123" || !*d.Albums[0].Explicit || d.Albums[1].Type != "single" || d.Albums[1].Title != "Hit" {
		t.Fatalf("apple: %+v %v", d, err)
	}

	// Apple fails: Spotify's albums, kept an hour.
	g2 := newRig(t)
	g2.stored["artist:"+artistB] = `{"source_data":{"release-groups":[]}}`
	g2.artist(artistB, "New One", "https://music.apple.com/us/artist/222", "https://open.spotify.com/artist/spBBBBBBBBBBBBBBBBBBBB")
	g2.f.set("/v1/catalog/us/artists/222", 503)
	g2.f.set("/spotify/v1/artists/spBBBBBBBBBBBBBBBBBBBB/albums", map[string]any{"items": []any{map[string]any{"name": "Debut", "album_type": "album", "total_tracks": 9, "release_date": "2026-10-23"}}})
	d, err = g2.s.Discography(ctx, artistB)
	if err != nil || d.Source != "spotify" || d.Albums[0].Explicit != nil || d.Albums[0].TrackCount != 9 {
		t.Fatalf("spotify after apple failed: %+v %v", d, err)
	}
	var until int64
	_ = g2.s.db.QueryRow(`SELECT until FROM answers WHERE k = ?`, "disc:"+artistB).Scan(&until)
	if time.Until(time.Unix(until, 0)) > 2*time.Hour {
		t.Fatal("kept a week after Apple failed")
	}
	// No stored MusicBrainz answer: ask again later.
	if _, err := g2.s.Discography(ctx, artistA); !errors.Is(err, ErrLater) {
		t.Fatalf("not stored: %v", err)
	}
}
