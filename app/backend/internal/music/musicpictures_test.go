package music

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	picAlbum2 = "54be6365-1bc0-4825-b87e-505a2b48fade"
	picArtist = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
)

func squareJPEG(t *testing.T, px int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, px, px)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Every music answer carries the records of the pictures it shows, read in
// one batch call for the whole row and kept, so a second answer asks
// nothing; the "pictures" map comes first in the answer.
func TestMusicAnswerCarriesPictureRecords(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var mu sync.Mutex
	var asked [][2][]string
	a := New(Server{Version: "test", ImageBase: "https://api.example.test/v1/img"})
	a.s.MusicPictures = func(_ context.Context, artists, albums []string) (metadata.MusicPictures, error) {
		calls.Add(1)
		mu.Lock()
		asked = append(asked, [2][]string{artists, albums})
		mu.Unlock()
		return metadata.MusicPictures{
			Artists: map[string]metadata.ArtistPictures{picArtist: {Photo: &metadata.PictureRecord{V: "p1", W: 1200, H: 1200, Color: "#112233"}, Logo: &metadata.PictureRecord{V: "l1", W: 800, H: 310}}},
			Albums:  map[string]metadata.AlbumPictures{picAlbum: {Cover: &metadata.PictureRecord{V: "c1", W: 1200, H: 1200, Color: "#a0b0c0"}}},
		}, nil
	}
	row := map[string]any{"rows": []MusicRow{{ID: "r", Items: []MusicCard{
		{Kind: "album", MBID: picAlbum, Image: musicArtPrefix + "album/" + picAlbum, ArtistImage: musicArtPrefix + "artist/" + picArtist},
		{Kind: "album", MBID: picAlbum2, Image: musicArtPrefix + "album/" + picAlbum2},
	}}}}
	b, _ := json.Marshal(row)
	out := a.WithPictures(context.Background(), b, musicArtPrefix+"logo/"+picArtist)
	if !bytes.HasPrefix(out, []byte(`{"pictures":{`)) {
		t.Fatalf("answer %s", out)
	}
	var got struct {
		Pictures map[string]metadata.PictureRecord `json:"pictures"`
		Rows     []MusicRow                        `json:"rows"`
	}
	if err := json.Unmarshal(out, &got); err != nil || len(got.Rows) != 1 {
		t.Fatalf("%v: %s", err, out)
	}
	if got.Pictures["album:"+picAlbum].Color != "#a0b0c0" || got.Pictures["artist:"+picArtist].V != "p1" || got.Pictures["logo:"+picArtist].W != 800 {
		t.Fatalf("pictures %+v", got.Pictures)
	}
	if _, ok := got.Pictures["album:"+picAlbum2]; ok {
		t.Fatal("a record the server has not stored was made up")
	}
	if calls.Load() != 1 || len(asked[0][0]) != 1 || len(asked[0][1]) != 2 {
		t.Fatalf("%d calls, asked %v: want one batch for the row", calls.Load(), asked)
	}
	// Known records are not asked again; the album still missing a cover is
	// asked again only after a short while.
	a.WithPictures(context.Background(), b)
	if calls.Load() != 1 {
		t.Fatalf("records asked again: %d calls", calls.Load())
	}
	// No picture paths: the answer is left alone and nothing is asked.
	if plain := a.WithPictures(context.Background(), []byte(`{"kid":true}`)); string(plain) != `{"kid":true}` || calls.Load() != 1 {
		t.Fatalf("plain answer %s, %d calls", plain, calls.Load())
	}
	if empty := a.WithPictures(context.Background(), []byte(`{}`), musicArtPrefix+"album/"+picAlbum); string(empty) != `{"pictures":{"album:`+picAlbum+`":{"v":"c1","w":1200,"h":1200,"color":"#a0b0c0"}}}` {
		t.Fatalf("empty object %s", empty)
	}
}

// route: GET /api/v1/music-trending/art/{kind}/{mbid}
// The picture route asks the server's WebP at the stored version and keeps
// that answer a year; 250 is made here from the 500 (the server is never
// asked for 250); a big size not made yet (X-Metadata-Partial) is never
// kept, and the 500 stands in briefly.
func TestMusicPictureRouteVersionsSizes(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	var mu sync.Mutex
	var paths []string
	pic := squareJPEG(t, 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "image/webp")
		switch r.URL.Path {
		case "/v1/img/album/" + picAlbum + "/500.webp":
			_, _ = w.Write(pic)
		case "/v1/img/album/" + picAlbum + "/1200.webp":
			w.Header().Set("X-Metadata-Partial", "true")
			_, _ = w.Write(pic)
		case "/v1/img/artist/" + picArtist + "/500.webp":
			w.Header().Set("X-Metadata-Placeholder", "true")
			_, _ = w.Write(pic)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/v1/img/"}, HTTP: srv.Client()}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art}, ImageBase: srv.URL + "/v1/img", Version: "test"})
	a.rememberNames([]nameKey{{kind: "album", mbid: picAlbum, name: "Arirang"}})
	h := httptest.NewServer(a.Handler())
	defer h.Close()
	get := func(q string) (int, string, []byte) {
		res, err := http.Get(h.URL + musicArtPrefix + q)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Cache-Control"), b
	}
	// Not on disk yet: 204 at once, fetched behind the answer.
	if code, _, _ := get("album/" + picAlbum + "?size=500&pv=c1"); code != 204 {
		t.Fatalf("first ask %d, want 204 while it is fetched", code)
	}
	a.music.prefetching.Wait()
	code, cc, body := get("album/" + picAlbum + "?size=500&pv=c1")
	if code != 200 || !strings.Contains(cc, "immutable") || !bytes.Equal(body, pic) {
		t.Fatalf("versioned card %d %q", code, cc)
	}
	code, cc, body = get("album/" + picAlbum + "?size=250&pv=c1")
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(body))
	if code != 200 || err != nil || cfg.Width != 250 || !strings.Contains(cc, "immutable") {
		t.Fatalf("thumbnail %d %q %dx%d %v", code, cc, cfg.Width, cfg.Height, err)
	}
	code, cc, _ = get("album/" + picAlbum + "?size=1200&pv=c1")
	if code != 200 || cc != "private, max-age=60" {
		t.Fatalf("big not made yet: %d %q, want the 500 for a minute", code, cc)
	}
	if _, ok := art.Path(library.ArtworkName(srv.URL + "/v1/img/album/" + picAlbum + "/1200.webp?v=c1")); ok {
		t.Fatal("a partial answer was kept as the big size")
	}
	if code, _, _ := get("artist/" + picArtist); code != 204 {
		t.Fatalf("artist placeholder: %d, want 204", code)
	}
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(paths, " ")
	if strings.Contains(joined, "/250") || strings.Contains(joined, ".jpg") {
		t.Fatalf("asked the server %s", joined)
	}
	if !strings.Contains(joined, "/v1/img/album/"+picAlbum+"/500.webp?v=c1") {
		t.Fatalf("the version was not passed on: %s", joined)
	}
	if n := strings.Count(joined, "/500.webp?v=c1"); n != 1 {
		t.Fatalf("the 500 was fetched %d times, want once", n)
	}
}

// artSize: covers and photos default to the card size; wide to the hero.
func TestArtSize(t *testing.T) {
	t.Parallel()
	for _, c := range [][3]string{{"album", "", "500"}, {"album", "250", "250"}, {"artist", "1200", "1200"}, {"artist", "999", "500"},
		{"wide", "", "1920"}, {"wide", "500", "960"}, {"wide", "960", "960"}, {"wide", "250", "1920"}, {"logo", "500", "1200"}} {
		if got := ArtSize(c[0], c[1]); got != c[2] {
			t.Errorf("ArtSize(%s, %q) = %s, want %s", c[0], c[1], got, c[2])
		}
	}
}

// A page's warm asks its records in one batch call, and each warmed item
// waits for that answer instead of asking for itself.
func TestWarmAsksRecordsOncePerRow(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	a := New(Server{Version: "test", ImageBase: "https://api.example.test/v1/img"})
	release := make(chan struct{})
	a.s.MusicPictures = func(_ context.Context, artists, albums []string) (metadata.MusicPictures, error) {
		calls.Add(1)
		<-release
		return metadata.MusicPictures{Albums: map[string]metadata.AlbumPictures{picAlbum: {Cover: &metadata.PictureRecord{V: "c1", W: 500, H: 500}}}}, nil
	}
	var mu sync.Mutex
	var seen []string
	done := make(chan struct{}, 3)
	a.SetArtWarmDo(func(_ context.Context, k ArtWarmKey) {
		mu.Lock()
		seen = append(seen, k.MBID+"="+a.PictureVersion("album", k.MBID))
		mu.Unlock()
		done <- struct{}{}
	})
	a.warmArtItems([]nameKey{{kind: "album", mbid: picAlbum}, {kind: "album", mbid: picAlbum2}, {kind: "artist", mbid: picArtist}})
	close(release)
	for range 3 {
		<-done
	}
	if calls.Load() != 1 {
		t.Fatalf("%d record calls for one row, want 1", calls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(seen, " "), picAlbum+"=c1") {
		t.Fatalf("the warm ran before its records: %v", seen)
	}
}

// A music answer carries an ETag; the same answer asked with it is a 304
// with no body.
func TestMusicJSONETag(t *testing.T) {
	t.Parallel()
	a := New(Server{Version: "test"})
	w := httptest.NewRecorder()
	a.WriteMusicJSON(w, httptest.NewRequest("GET", "/x", nil), map[string]any{"rows": []int{1, 2, 3}})
	tag := w.Header().Get("ETag")
	if w.Code != 200 || tag == "" {
		t.Fatalf("first answer %d, ETag %q", w.Code, tag)
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("If-None-Match", "W/"+tag)
	w = httptest.NewRecorder()
	a.WriteMusicJSON(w, r, map[string]any{"rows": []int{1, 2, 3}})
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("revisit %d with %d bytes, want 304", w.Code, w.Body.Len())
	}
	w = httptest.NewRecorder()
	a.WriteMusicJSON(w, r, map[string]any{"rows": []int{4}})
	if w.Code != 200 {
		t.Fatalf("changed answer %d, want 200", w.Code)
	}
}
