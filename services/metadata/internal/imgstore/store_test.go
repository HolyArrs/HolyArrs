package imgstore

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	mrand "math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	artistID = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	albumID  = "b1392450-e666-3926-a536-22c65f834433"
)

// picture is a sharp, busy test picture: random blocks (1/128 of its
// width, so it stays sharp at any size) over a gradient.
func picture(w, h int, seed int64) image.Image {
	rnd := mrand.New(mrand.NewSource(seed))
	block := max(1, w/128)
	noise := make([]uint8, (w/block+1)*(h/block+1))
	for i := range noise {
		noise[i] = uint8(rnd.Intn(120))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			n := noise[(y/block)*(w/block+1)+x/block]
			img.SetNRGBA(x, y, color.NRGBA{uint8(x*255/w)/2 + n, uint8(y*255/h)/2 + n/2, 90 + n/3, 255})
		}
	}
	return img
}

func flat(w, h int, c uint8) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c, c, c, 255
	}
	return img
}

// fake is every source at once: the APIs answer from per-test maps, and
// /pic/{w}x{h}/{name}.jpg answers a picture of that size.
type fake struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex
	// api answers by path (and query "term" or "q" when set): JSON body,
	// or a status code as an int.
	api  map[string]any
	hits map[string]int
	// picStatus forces a status for picture paths containing a key.
	picStatus map[string]int
	pics      map[string]image.Image
	slow      time.Duration
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, api: map[string]any{}, hits: map[string]int{}, picStatus: map[string]int{}, pics: map[string]image.Image{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	slow := f.slow
	f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/pic/") {
		time.Sleep(slow)
		f.mu.Lock()
		for k, st := range f.picStatus {
			if strings.Contains(r.URL.Path, k) {
				f.mu.Unlock()
				w.WriteHeader(st)
				return
			}
		}
		img := f.pics[r.URL.Path]
		f.mu.Unlock()
		if img == nil {
			var wd, ht int
			var name string
			if _, err := fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/pic/"), "%dx%d/%s", &wd, &ht, &name); err != nil {
				w.WriteHeader(404)
				return
			}
			img = picture(wd, ht, int64(len(name)))
		}
		w.Header().Set("ETag", `"`+r.URL.Path+`"`)
		if r.Header.Get("If-None-Match") == `"`+r.URL.Path+`"` {
			w.WriteHeader(304)
			return
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		_, _ = w.Write(b.Bytes())
		return
	}
	k := r.URL.Path
	if q := r.URL.Query(); q.Get("term") != "" {
		k += "?" + q.Get("term")
	} else if q.Get("q") != "" {
		k += "?" + q.Get("q")
	}
	f.mu.Lock()
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
}

func (f *fake) set(k string, v any) {
	f.mu.Lock()
	f.api[k] = v
	f.mu.Unlock()
}

func (f *fake) hit(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func (f *fake) pic(w, h int, name string) string {
	return f.srv.URL + "/pic/" + strconv.Itoa(w) + "x" + strconv.Itoa(h) + "/" + name + ".jpg"
}

// artwork is an Apple artwork answer at the fake ({w}x{h} template).
func (f *fake) artwork(name string, w, h int) map[string]any {
	return map[string]any{"url": f.srv.URL + "/pic/{w}x{h}/" + name + ".jpg", "width": w, "height": h}
}

type rig struct {
	f      *fake
	r      *Resolver
	s      *Store
	info   map[string]Subject
	bigs   []Key
	videos []string
}

func newRig(t *testing.T) *rig {
	f := newFake(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	g := &rig{f: f, info: map[string]Subject{}}
	g.r = &Resolver{HTTP: f.srv.Client(), AppleMusic: &AppleMusic{TeamID: "T", KeyID: "K", Key: key, Base: f.srv.URL},
		CAA: f.srv.URL + "/caa", Deezer: f.srv.URL + "/deezer", SpotifyAPI: f.srv.URL + "/spotify", SpotifyAuth: f.srv.URL + "/spotify-auth",
		YTImg: f.srv.URL + "/yt", Keys: Keys{SpotifyID: "id", SpotifySecret: "secret"}, Limits: NewLimits(nil),
		Info:   func(_ context.Context, kind, mbid string) Subject { return g.info[kind+":"+mbid] },
		Videos: func(context.Context, string) []string { return g.videos }}
	f.set("/spotify-auth", map[string]any{"access_token": "tok", "expires_in": 3600})
	var err error
	g.s, err = Open(Options{Dir: t.TempDir(), Chain: g.r.Chain, HTTP: f.srv.Client(), Logf: func(string, ...any) {},
		Known: func(_ context.Context, kind, mbid string) bool {
			if kind != KindAlbum && kind != KindArtist && kind != KindWide {
				return true // these go by ID only
			}
			return g.info[kind+":"+mbid].Known
		},
		WantBig: func(kind, mbid string) { g.bigs = append(g.bigs, Key{kind, mbid}) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.s.Close() })
	return g
}

func (g *rig) album(name, title, country string) {
	g.info[KindAlbum+":"+albumID] = Subject{Known: true, Name: name, Title: title, Country: country}
}

func (g *rig) artist(name string, links ...string) {
	g.info[KindArtist+":"+artistID] = Subject{Known: true, Name: name, Links: links}
	g.info[KindWide+":"+artistID] = g.info[KindArtist+":"+artistID]
}

func appleAlbums(f *fake, name, artist string, w int) map[string]any {
	return map[string]any{"results": map[string]any{"albums": map[string]any{"data": []any{
		map[string]any{"attributes": map[string]any{"name": name, "artistName": artist, "artwork": f.artwork("apple-"+name, w, w)}}}}}}
}

// A cover whose source has a bigger address gets its big size at store
// time; a fetch someone waits on (BigLater) queues it instead.
func TestAlbumCoverAppleFirstBigAtStoreTime(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde (Deluxe)", "Frank Ocean", 3000))
	ctx := context.Background()
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, ok := g.s.Record(ctx, KindAlbum, albumID)
	if !ok || rec.Provider != "applemusic" || !rec.Has(SlotCard) || !rec.Has(SlotBig) || rec.Width != 1200 || rec.Color == "" {
		t.Fatalf("record %+v", rec)
	}
	if !strings.Contains(rec.SourceURL, "/500x500/") || !strings.Contains(rec.BigURL, "/1200x1200/") {
		t.Fatalf("Apple is asked at the slot size: %s %s", rec.SourceURL, rec.BigURL)
	}
	if len(g.bigs) != 0 || g.f.hit("/deezer/search/album") != 0 || g.f.hit("/caa/release-group/"+albumID+"/front-500") != 0 {
		t.Fatalf("big made at once, nothing queued, later sources never asked: bigs %v", g.bigs)
	}
	p, _, final, ok := g.s.Served(ctx, KindAlbum, albumID, "1200")
	if !ok || !final || !strings.HasSuffix(p, "1200.webp") {
		t.Fatalf("1200 served from its own file: %s %v", p, final)
	}
}

func TestBigLaterQueuesTheBigSize(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde (Deluxe)", "Frank Ocean", 3000))
	ctx := BigLater(context.Background())
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindAlbum, albumID)
	if rec.Has(SlotBig) || rec.Width != 500 || len(g.bigs) != 1 {
		t.Fatalf("big not queued: %+v bigs %v", rec, g.bigs)
	}
	if left := g.s.BigsLeft(ctx); left[KindAlbum] != 1 {
		t.Fatalf("bigs left %v", left)
	}
	if err := g.s.MakeBig(context.Background(), KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if rec, _ = g.s.Record(ctx, KindAlbum, albumID); !rec.Has(SlotBig) || rec.Width != 1200 {
		t.Fatalf("big made at 1200: %+v", rec)
	}
}

func TestBigNeverUpscaled(t *testing.T) {
	g := newRig(t)
	g.album("Low", "Things We Lost in the Fire", "")
	// The original is 800 px: Apple's template is never asked past it.
	g.f.set("/v1/catalog/us/search?Low Things We Lost in the Fire", appleAlbums(g.f, "Things We Lost in the Fire", "Low", 800))
	ctx := context.Background()
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindAlbum, albumID)
	if !strings.Contains(rec.BigURL, "/800x800/") {
		t.Fatalf("big address capped at the original: %s", rec.BigURL)
	}
	if err := g.s.MakeBig(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ = g.s.Record(ctx, KindAlbum, albumID)
	if rec.Width != 800 || rec.Height != 800 {
		t.Fatalf("big stays at the source's 800, got %dx%d", rec.Width, rec.Height)
	}
}

func TestAlbumAppleHomeStoreThenDeezerThenArchive(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.album("米津玄師", "STRAY SHEEP", "JP")
	g.f.set("/v1/catalog/jp/search?米津玄師 STRAY SHEEP", appleAlbums(g.f, "STRAY SHEEP", "米津玄師", 3000))
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindAlbum, albumID); rec.Provider != "applemusic" || g.f.hit("/v1/catalog/us/search") != 1 {
		t.Fatalf("US store first, then JP: %+v", rec)
	}

	g = newRig(t)
	g.album("Low", "Double Negative", "US")
	g.f.set(`/deezer/search/album?artist:"Low" album:"Double Negative"`, map[string]any{"data": []any{
		map[string]any{"title": "Double Negative", "cover_xl": "https://e-cdns-images.dzcdn.net/images/cover/abc/1000x1000-000000-80-0-0.jpg", "artist": map[string]any{"name": "Low"}}}})
	// The Deezer CDN address is rewritten to the fake host for the test.
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindAlbum, albumID)
	if rec.Provider != "deezer" || !strings.Contains(rec.SourceURL, "/500x500-") || !strings.Contains(rec.BigURL, "/1000x1000-") {
		t.Fatalf("Deezer at 500 and 1000: %+v", rec)
	}

	g = newRig(t)
	g.album("Low", "I Could Live in Hope", "US")
	g.f.set("/caa/release-group/"+albumID, map[string]any{"images": []any{map[string]any{"front": true}}})
	g.f.pics["/caa/release-group/"+albumID+"/front-500"] = picture(500, 500, 3)
	g.s.o.HTTP = rewrite(g.f, "")
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindAlbum, albumID); rec.Provider != "caa" || !strings.HasSuffix(rec.BigURL, "/front-1200") {
		t.Fatalf("the archive last: %+v", rec)
	}
}

// rewrite answers a client whose picture asks go to the fake: a CDN host
// is sent there, and /caa/ picture paths are served as pictures.
func rewrite(f *fake, host string) *http.Client {
	return &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		u := *r.URL
		if host != "" && u.Host == host {
			u.Scheme, u.Host = "http", strings.TrimPrefix(f.srv.URL, "http://")
			u.Path = "/pic/" + sizeOf(u.Path) + "/dz.jpg"
		}
		if strings.HasPrefix(u.Path, "/caa/") {
			u.Path = "/pic" + u.Path
			if strings.HasSuffix(u.Path, "-500") {
				u.Path = "/pic/500x500/caa.jpg"
			} else {
				u.Path = "/pic/1200x1200/caa.jpg"
			}
		}
		r2 := r.Clone(r.Context())
		r2.URL = &u
		r2.Host = u.Host
		return f.srv.Client().Transport.RoundTrip(r2)
	})}
}

func sizeOf(p string) string {
	for _, part := range strings.Split(p, "/") {
		if i := strings.Index(part, "x"); i > 0 {
			if n, err := strconv.Atoi(part[:i]); err == nil {
				return strconv.Itoa(n) + "x" + strconv.Itoa(n)
			}
		}
	}
	return "500x500"
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNotFoundIsKeptBusyIsNot(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.album("Nobody", "Nothing", "")
	err := g.s.Fetch(ctx, KindAlbum, albumID)
	if !errors.Is(err, ErrNoPicture) {
		t.Fatalf("every source said not found: %v", err)
	}
	before := g.f.hit("/v1/catalog/us/search")
	if err := g.s.Fetch(ctx, KindAlbum, albumID); !errors.Is(err, ErrNoPicture) || g.f.hit("/v1/catalog/us/search") != before {
		t.Fatalf("a real not found is kept for a week (no new asks): %v", err)
	}

	for _, status := range []int{429, 503} {
		g = newRig(t)
		g.album("Busy", "Album", "")
		g.f.set("/v1/catalog/us/search?Busy Album", status)
		err := g.s.Fetch(ctx, KindAlbum, albumID)
		if !errors.Is(err, ErrRetry) {
			t.Fatalf("%d is ask again later, got %v", status, err)
		}
		if g.s.missed(ctx, KindAlbum, albumID) {
			t.Fatalf("%d must never be stored as missing", status)
		}
	}

	// A picture host that is down is busy, not missing.
	g = newRig(t)
	g.album("Down", "Host", "")
	g.f.set("/v1/catalog/us/search?Down Host", appleAlbums(g.f, "Host", "Down", 3000))
	g.f.picStatus["apple-Host"] = 502
	if err := g.s.Fetch(ctx, KindAlbum, albumID); !errors.Is(err, ErrRetry) || g.s.missed(ctx, KindAlbum, albumID) {
		t.Fatalf("a 502 picture host is ask again later: %v", err)
	}
}

func TestStoredPictureKeptWhenAnEarlierSourceIsBusy(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde", "Frank Ocean", 3000))
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", 429)
	g.r.Limits = NewLimits(nil)
	g.f.set("/caa/release-group/"+albumID, map[string]any{"images": []any{map[string]any{"front": true}}})
	g.f.pics["/caa/release-group/"+albumID+"/front-500"] = picture(500, 500, 9)
	if err := g.s.Fetch(ctx, KindAlbum, albumID); !errors.Is(err, ErrRetry) {
		t.Fatalf("Apple busy on a re-check: ask again later, got %v", err)
	}
	if rec, _ := g.s.Record(ctx, KindAlbum, albumID); rec.Provider != "applemusic" {
		t.Fatalf("the stored Apple cover stays: %+v", rec)
	}
}

func TestArtistPhotoRealAppleThenSpotifyThenDeezer(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.artist("Drake", "https://music.apple.com/us/artist/drake/271256")
	// Apple shows album art for this artist: a miss.
	g.f.set("/v1/catalog/us/artists/271256", map[string]any{"data": []any{map[string]any{"id": "271256", "attributes": map[string]any{"name": "Drake",
		"artwork": map[string]any{"url": g.f.srv.URL + "/pic/{w}x{h}/Music115/cover.jpg", "width": 3000, "height": 3000}}}}})
	g.f.set("/spotify/v1/search?Drake", map[string]any{"artists": map[string]any{"items": []any{map[string]any{"name": "Drake",
		"images": []any{map[string]any{"url": g.f.pic(640, 640, "spotify"), "width": 640, "height": 640}, map[string]any{"url": g.f.pic(160, 160, "spotify"), "width": 160, "height": 160}}}}}})
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindArtist, artistID)
	if rec.Provider != "spotify" || !strings.Contains(rec.SourceURL, "640x640") || !rec.Has(SlotBig) || rec.Width != 640 {
		t.Fatalf("Spotify's 640 is the source; big is its 640: %+v", rec)
	}
	if d := rec.Recheck.Sub(rec.Fetched); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("Spotify is re-checked every 30 days, got %v", d)
	}

	// A real Apple photo wins.
	g = newRig(t)
	g.artist("Drake", "https://music.apple.com/us/artist/drake/271256")
	g.f.set("/v1/catalog/us/artists/271256", map[string]any{"data": []any{map[string]any{"id": "271256", "attributes": map[string]any{"name": "Drake",
		"artwork": map[string]any{"url": g.f.srv.URL + "/pic/{w}x{h}/AMCArtistImages/drake.jpg", "width": 2400, "height": 2400}}}}})
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindArtist, artistID); rec.Provider != "applemusic" || g.f.hit("/spotify/v1/search") != 0 {
		t.Fatalf("a real Apple photo first: %+v", rec)
	}

	// Deezer last; its blank stand-in and a grey default picture are refused.
	g = newRig(t)
	g.r.Keys = Keys{}
	g.artist("Low", "https://www.deezer.com/artist/1234")
	g.f.set("/deezer/artist/1234", map[string]any{"name": "Low", "picture_xl": "https://e-cdns-images.dzcdn.net/images/artist//1000x1000-000000-80-0-0.jpg"})
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindArtist, artistID); !errors.Is(err, ErrNoPicture) {
		t.Fatalf("an empty Deezer hash is no picture: %v", err)
	}
	g = newRig(t)
	g.r.Keys = Keys{}
	g.artist("Low", "https://www.deezer.com/artist/1234")
	g.f.set("/deezer/artist/1234", map[string]any{"name": "Low", "picture_xl": "https://e-cdns-images.dzcdn.net/images/artist/9f1c/1000x1000-000000-80-0-0.jpg"})
	g.f.pics["/pic/500x500/dz.jpg"] = flat(500, 500, 0xd8)
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindArtist, artistID); !errors.Is(err, ErrNoPicture) {
		t.Fatalf("Deezer's grey default picture is refused: %v", err)
	}
}

func TestQualityFloorMovesOn(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.album("Tiny", "Cover", "")
	g.f.set("/v1/catalog/us/search?Tiny Cover", appleAlbums(g.f, "Cover", "Tiny", 250))
	g.f.set("/caa/release-group/"+albumID, map[string]any{"images": []any{map[string]any{"front": true}}})
	g.f.pics["/caa/release-group/"+albumID+"/front-500"] = picture(500, 500, 4)
	g.s.o.HTTP = rewrite(g.f, "")
	if err := g.s.Fetch(ctx, KindAlbum, albumID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindAlbum, albumID); rec.Provider != "caa" {
		t.Fatalf("a 250 px Apple cover is under the floor; the archive fills: %+v", rec)
	}
}

func TestSingleFlight(t *testing.T) {
	g := newRig(t)
	g.album("Frank Ocean", "Blonde", "")
	g.f.set("/v1/catalog/us/search?Frank Ocean Blonde", appleAlbums(g.f, "Blonde", "Frank Ocean", 3000))
	g.f.slow = 150 * time.Millisecond
	var wg sync.WaitGroup
	var failed atomic.Int32
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.s.Fetch(context.Background(), KindAlbum, albumID) != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() != 0 || g.f.hit("/pic/500x500/apple-Blonde.jpg") != 1 || g.f.hit("/v1/catalog/us/search") != 1 {
		t.Fatalf("10 asks at once = 1 fetch: pictures %d searches %d failed %d", g.f.hit("/pic/500x500/apple-Blonde.jpg"), g.f.hit("/v1/catalog/us/search"), failed.Load())
	}
}

func TestWideAndLogo(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.artist("Drake", "https://music.apple.com/us/artist/drake/271256")
	logo := image.NewNRGBA(image.Rect(0, 0, 800, 310))
	for y := 100; y < 200; y++ {
		for x := 100; x < 700; x++ {
			logo.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	g.f.pics["/pic/800x310/logo.jpg"] = logo
	g.r.Fanart = func(context.Context, string) ([]byte, error) {
		return json.Marshal(map[string]any{"backgrounds": []string{g.f.pic(3840, 2160, "fanart")}, "logo": g.f.pic(800, 310, "logo")})
	}
	if err := g.s.Fetch(ctx, KindWide, artistID); err != nil {
		t.Fatal(err)
	}
	rec, _ := g.s.Record(ctx, KindWide, artistID)
	if rec.Provider != "fanart" || !rec.Has(SlotWideRow) || !rec.Has(SlotWideHero) || rec.Width != 1920 || rec.Height != 1080 {
		t.Fatalf("fanart.tv background at 960 and 1920: %+v", rec)
	}
	if err := g.s.Fetch(ctx, KindLogo, artistID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindLogo, artistID); rec.Provider != "fanart" || !rec.Has(SlotLogo) || rec.Width != 800 {
		t.Fatalf("logo stored as it comes, PNG: %+v", rec)
	}

	// No fanart.tv: Apple's music-video still, the artist's own first.
	g = newRig(t)
	g.artist("Drake", "https://music.apple.com/us/artist/drake/271256")
	g.f.set("/v1/catalog/us/artists/271256", map[string]any{"data": []any{map[string]any{"id": "271256", "attributes": map[string]any{"name": "Drake", "artwork": g.f.artwork("x", 10, 10)}}}})
	g.f.set("/v1/catalog/us/artists/271256/music-videos", map[string]any{"data": []any{
		map[string]any{"attributes": map[string]any{"artistName": "Rihanna & Drake", "artwork": g.f.artwork("feat", 3840, 2160)}},
		map[string]any{"attributes": map[string]any{"artistName": "Drake", "artwork": g.f.artwork("own", 3840, 2160)}}}})
	if err := g.s.Fetch(ctx, KindWide, artistID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindWide, artistID); rec.Provider != "applemusic" || !strings.Contains(rec.SourceURL, "/1920x1080/own") {
		t.Fatalf("Apple still of the artist's own video at 1920x1080: %+v", rec)
	}

	// Neither: a YouTube still, through our server.
	g = newRig(t)
	g.artist("Low")
	g.videos = []string{"dQw4w9WgXcQ"}
	g.f.pics["/yt/vi/dQw4w9WgXcQ/maxresdefault.jpg"] = picture(1280, 720, 5)
	g.s.o.HTTP = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		if strings.HasPrefix(r2.URL.Path, "/yt/") {
			r2.URL.Path = "/pic/1280x720/yt.jpg"
		}
		return g.f.srv.Client().Transport.RoundTrip(r2)
	})}
	if err := g.s.Fetch(ctx, KindWide, artistID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindWide, artistID); rec.Provider != "youtube" || rec.Width != 1280 || rec.Recheck.Sub(rec.Fetched) > 31*24*time.Hour {
		t.Fatalf("YouTube still stored, 1280 wide, re-checked in 30 days: %+v", rec)
	}
}

func TestPurgeProviderAndRefillFromLinks(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.artist("Drake")
	g.f.set("/spotify/v1/search?Drake", map[string]any{"artists": map[string]any{"items": []any{map[string]any{"name": "Drake",
		"images": []any{map[string]any{"url": g.f.pic(640, 640, "spotify"), "width": 640, "height": 640}}}}}})
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatal(err)
	}
	// Files lost: refill from the stored links rebuilds them.
	if err := os.RemoveAll(g.s.Dir(KindArtist, artistID)); err != nil {
		t.Fatal(err)
	}
	var refilled []Key
	if err := g.s.Refill(ctx, true, func(k Key, err error) {
		if err != nil {
			t.Fatal(err)
		}
		refilled = append(refilled, k)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := g.s.Served(ctx, KindArtist, artistID, "500"); !ok || len(refilled) != 1 {
		t.Fatalf("refilled %v", refilled)
	}
	keys, err := g.s.Purge(ctx, "spotify")
	if err != nil || len(keys) != 1 || keys[0] != (Key{KindArtist, artistID}) {
		t.Fatalf("purge answers what to fill again: %v %v", keys, err)
	}
	if _, ok := g.s.Record(ctx, KindArtist, artistID); ok {
		t.Fatal("purged picture is gone")
	}
	// Filled again with Spotify off: the next source (Deezer).
	g.r.On = func(p string) bool { return p != "spotify" }
	g.f.set("/deezer/search/artist?Drake", map[string]any{"data": []any{map[string]any{"name": "Drake", "picture_xl": "https://e-cdns-images.dzcdn.net/images/artist/77aa/1000x1000-000000-80-0-0.jpg"}}})
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindArtist, artistID); rec.Provider != "deezer" {
		t.Fatalf("filled from the next source: %+v", rec)
	}
}

func TestUnknownSubjectWaits(t *testing.T) {
	g := newRig(t)
	// No stored answer: name sources cannot run; the archive alone has no
	// front. Ask again once the answer is there, never a miss.
	if err := g.s.Fetch(context.Background(), KindAlbum, albumID); !errors.Is(err, ErrRetry) || g.s.missed(context.Background(), KindAlbum, albumID) {
		t.Fatalf("unknown subject: %v", err)
	}
}

// A soft official portrait (a smooth studio background) is a photo; only a
// wide background is checked for blur.
func TestSoftPhotoAcceptedBlurryWideRefused(t *testing.T) {
	soft := image.NewNRGBA(image.Rect(0, 0, 500, 500))
	for y := range 500 {
		for x := range 500 {
			soft.SetNRGBA(x, y, color.NRGBA{uint8(80 + x/5), uint8(60 + y/6), 120, 255})
		}
	}
	if g := CheckGate(KindArtist, soft); !g.OK {
		t.Fatalf("soft photo refused: %+v", g)
	}
	wide := Resize(soft, 1920)
	big := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
	for y := range 1080 {
		for x := range 1920 {
			big.Set(x, y, wide.At(x*500/1920, y*500/1080))
		}
	}
	if g := CheckGate(KindWide, big); g.OK || g.Reason != "blur" {
		t.Fatalf("a blurry background is refused: %+v", g)
	}
}

// The big-size backfill takes artist photos first, then covers, most
// viewed first within each.
func TestBigsDueArtistsFirst(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	ins := func(kind, id string, views int) {
		if _, err := g.s.db.Exec(`INSERT INTO images(kind,mbid,source,source_url,big_url,source_etag,license,author,version,width,height,color,sizes,bytes,fetched,checked,recheck,views) VALUES(?,?,'x','u','big','','','','v',500,500,'',?,1,0,0,0,?)`, kind, id, "500", views); err != nil {
			t.Fatal(err)
		}
	}
	ins(KindAlbum, "a1", 900)
	ins(KindArtist, "p1", 1)
	ins(KindArtist, "p2", 5)
	ins(KindAlbum, "a2", 3)
	got := g.s.BigsDue(ctx, 10)
	want := []string{"p2", "p1", "a1", "a2"}
	if len(got) != 4 {
		t.Fatalf("%v", got)
	}
	for i, k := range got {
		if k.ID != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

// A paused or rate-limited source is skipped, not waited on: the next
// source's picture is stored, tagged for an upgrade from the skipped one;
// once it answers again, a fetch puts its picture in. A stored picture is
// never swapped for a later source while a better one is paused.
func TestPausedSourceSkippedAndUpgradedLater(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.artist("Drake", "https://music.apple.com/us/artist/drake/271256")
	g.f.set("/v1/catalog/us/artists/271256", map[string]any{"data": []any{map[string]any{"id": "271256", "attributes": map[string]any{"name": "Drake",
		"artwork": map[string]any{"url": g.f.srv.URL + "/pic/{w}x{h}/AMCArtistImages/drake.jpg", "width": 2400, "height": 2400}}}}})
	g.f.set("/spotify/v1/search?Drake", map[string]any{"artists": map[string]any{"items": []any{map[string]any{"name": "Drake",
		"images": []any{map[string]any{"url": g.f.pic(640, 640, "spotify"), "width": 640, "height": 640}}}}}})
	g.r.Limits.Backoff("applemusic", time.Hour)
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatalf("a paused Apple held the chain: %v", err)
	}
	rec, _ := g.s.Record(ctx, KindArtist, artistID)
	if rec.Provider != "spotify" || rec.Upgrade != "applemusic" || rec.Recheck.Sub(rec.Fetched) > UpgradeRecheck+time.Minute {
		t.Fatalf("not skipped and tagged: %+v", rec)
	}
	// Spotify paused too on a re-check: the stored picture stays.
	g.r.Limits.Backoff("spotify", time.Hour)
	_ = g.s.Fetch(ctx, KindArtist, artistID)
	if rec, _ := g.s.Record(ctx, KindArtist, artistID); rec.Provider != "spotify" {
		t.Fatalf("stored picture replaced: %+v", rec)
	}
	// Apple is back: its photo replaces the tagged one.
	g.r.Limits = NewLimits(nil)
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatal(err)
	}
	if rec, _ := g.s.Record(ctx, KindArtist, artistID); rec.Provider != "applemusic" || rec.Upgrade != "" {
		t.Fatalf("upgrade not made: %+v", rec)
	}
}

// Any source, not only Apple: a paused Spotify is skipped for Deezer, and
// Deezer's picture is tagged for a Spotify upgrade.
func TestPausedSpotifySkippedForDeezer(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.artist("Drake")
	g.r.Limits.Backoff("applemusic", time.Hour)
	g.r.Limits.Backoff("spotify", time.Hour)
	g.f.set("/deezer/search/artist?Drake", map[string]any{"data": []any{map[string]any{"id": 1, "name": "Drake",
		"picture_xl": "https://e-cdns-images.dzcdn.net/images/artist/9f1c/1000x1000-000000-80-0-0.jpg"}}})
	g.s.o.HTTP = rewrite(g.f, "e-cdns-images.dzcdn.net")
	if err := g.s.Fetch(ctx, KindArtist, artistID); err != nil {
		t.Fatalf("paused sources held the chain: %v", err)
	}
	rec, _ := g.s.Record(ctx, KindArtist, artistID)
	if rec.Provider != "deezer" || rec.Upgrade == "" {
		t.Fatalf("not skipped to Deezer and tagged: %+v", rec)
	}
}

// A paused source that may hold the only picture: nothing later has one,
// so the fetch asks again and keeps no miss.
func TestPausedSourceNoMiss(t *testing.T) {
	ctx := context.Background()
	g := newRig(t)
	g.artist("Drake")
	g.r.Limits.Backoff("spotify", time.Hour)
	g.f.set("/deezer/search/artist?Drake", map[string]any{"data": []any{}})
	if err := g.s.Fetch(ctx, KindArtist, artistID); !errors.Is(err, ErrRetry) || !errors.Is(err, ErrSourcePaused) {
		t.Fatalf("err %v, want a retry naming the paused source", err)
	}
	if g.s.Missed(ctx, KindArtist, artistID) {
		t.Fatal("a miss kept while Spotify was paused")
	}
}
