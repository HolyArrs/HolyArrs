package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

func noisy(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x*7 ^ y*13), uint8(x*y + 31), uint8(x ^ y), 255})
		}
	}
	return img
}

func TestTileJPEG(t *testing.T) {
	for _, sz := range [][2]int{{1920, 1080}, {1000, 1000}, {300, 900}} {
		b, err := TileJPEG(noisy(sz[0], sz[1]))
		if err != nil {
			t.Fatalf("%v: %v", sz, err)
		}
		if len(b) > tileMaxBytes {
			t.Fatalf("%v: %d bytes", sz, len(b))
		}
		img, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != tileW || img.Bounds().Dy() != tileH {
			t.Fatalf("%v: %v %v", sz, err, img.Bounds())
		}
		if _, grey := img.(*image.Gray); !grey {
			t.Fatalf("%v: not grey (%T)", sz, img)
		}
	}
	if _, err := TileJPEG(noisy(10, 4)); err == nil {
		t.Fatal("tiny picture rendered")
	}
}

type fakeCharts struct{ genres []musictrend.Genre }

func (f fakeCharts) Charts(context.Context, string, string, string, time.Time) ([]musictrend.ChartItem, bool) {
	return []musictrend.ChartItem{{Artist: "Some Artist", ArtistMBID: "11111111-2222-3333-4444-555555555555"}}, true
}
func (f fakeCharts) Genres(string) ([]musictrend.Genre, bool) { return f.genres, true }
func (f fakeCharts) Suggest(context.Context, string, int) []musictrend.Suggestion {
	return nil
}
func (f fakeCharts) List(string, string) ([]byte, string, bool) { return nil, "", false }

func genresAnswer(t *testing.T, s *Server) (string, []musictrend.Genre) {
	t.Helper()
	w := httptest.NewRecorder()
	s.serveMusicGenres(w, httptest.NewRequest("GET", "/v1/music/genres?region=us", nil))
	if w.Code != 200 {
		t.Fatalf("genres: %d", w.Code)
	}
	var out struct {
		Art    string             `json:"art"`
		Genres []musictrend.Genre `json:"genres"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Art, out.Genres
}

// With no picture source (fanart off, no Deezer) every tile is a flat hue.
func TestGenresFlatWithoutSources(t *testing.T) {
	s := &Server{opts: Options{Music: fakeCharts{genres: []musictrend.Genre{{Slug: "pop", Name: "Pop", Hue: 3}}}}}
	art, g := genresAnswer(t, s)
	if art != "flat" || len(g) != 1 || g[0].Art != "" {
		t.Fatalf("art %q genres %+v", art, g)
	}
	s.opts.GenreTiles = &GenreTiles{}
	if art, g = genresAnswer(t, s); art != "flat" || g[0].Art != "" {
		t.Fatalf("empty tiles: art %q genres %+v", art, g)
	}
	w := httptest.NewRecorder()
	s.serveMusicGenreTile(w, httptest.NewRequest("GET", "/v1/music/genres/tile/pop.jpg", nil))
	if w.Code != 404 {
		t.Fatalf("tile: %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.serveMusicGenreTile(w, httptest.NewRequest("GET", "/v1/music/genres/tile/Pop!.jpg", nil))
	if w.Code != 400 {
		t.Fatalf("bad slug: %d", w.Code)
	}
}

// Deezer's picture becomes a tile; the next answer links it.
func TestGenresTileFromDeezer(t *testing.T) {
	var pic bytes.Buffer
	_ = jpeg.Encode(&pic, noisy(800, 800), nil)
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/artist") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"name": "Some Artist", "picture_xl": srv.URL + "/pic/x.jpg"}}})
			return
		}
		_, _ = w.Write(pic.Bytes())
	}))
	defer srv.Close()
	tiles := &GenreTiles{DeezerURL: srv.URL, HTTP: srv.Client()}
	s := &Server{opts: Options{Music: fakeCharts{genres: []musictrend.Genre{{Slug: "pop", Name: "Pop", Hue: 3}}}, GenreTiles: tiles}}
	art, g := genresAnswer(t, s)
	if art != "photo" || g[0].Art != "" {
		t.Fatalf("first: %q %+v", art, g)
	}
	deadline := time.Now().Add(5 * time.Second)
	for tiles.Ready("pop") == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if _, g = genresAnswer(t, s); !strings.HasPrefix(g[0].Art, "/v1/music/genres/tile/pop.jpg?v=") {
		t.Fatalf("second: %+v", g)
	}
	w := httptest.NewRecorder()
	s.serveMusicGenreTile(w, httptest.NewRequest("GET", "/v1/music/genres/tile/pop.jpg?v=1", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || w.Body.Len() > tileMaxBytes {
		t.Fatalf("tile: %d %s %d", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
}

type twoArtists struct{ fakeCharts }

func (twoArtists) Charts(context.Context, string, string, string, time.Time) ([]musictrend.ChartItem, bool) {
	return []musictrend.ChartItem{{Artist: "Artist A"}, {Artist: "Artist B"}}, true
}

// Two genres led by one artist get two different photos.
func TestGenreTilesDistinctArtists(t *testing.T) {
	pics := map[string][]byte{}
	for i, n := range []string{"a", "b"} {
		var b bytes.Buffer
		_ = jpeg.Encode(&b, noisy(640+i*7, 480), nil)
		pics[n] = b.Bytes()
	}
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/artist") {
			q := r.URL.Query().Get("q")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"name": q, "picture_xl": srv.URL + "/pic/" + strings.ToLower(strings.TrimPrefix(q, "Artist ")) + ".jpg"}}})
			return
		}
		_, _ = w.Write(pics[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/pic/"), ".jpg")])
	}))
	defer srv.Close()
	tiles := &GenreTiles{DeezerURL: srv.URL, HTTP: srv.Client()}
	s := &Server{opts: Options{Music: twoArtists{fakeCharts{genres: []musictrend.Genre{{Slug: "pop"}, {Slug: "rock"}}}}, GenreTiles: tiles}}
	genresAnswer(t, s)
	deadline := time.Now().Add(5 * time.Second)
	for (tiles.Ready("pop") == "" || tiles.Ready("rock") == "") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if a, b := tiles.Ready("pop"), tiles.Ready("rock"); a == "" || b == "" || a == b {
		t.Fatalf("pop %q rock %q", a, b)
	}
}

func TestGenreTilesCapped(t *testing.T) {
	g := &GenreTiles{DeezerURL: "http://127.0.0.1:1", HTTP: &http.Client{Timeout: time.Second}}
	g.tiles, g.busy, g.used, g.slots = map[string]genreSet{}, map[string]bool{}, map[string]string{}, make(chan struct{}, tileWorkers)
	base := time.Now().Add(-time.Hour)
	for i := 0; i < tileMax; i++ {
		slug := fmt.Sprintf("g%d", i)
		g.tiles[slug] = genreSet{at: base.Add(time.Duration(i) * time.Second), photos: []genreTile{{jpeg: []byte{1}, artist: "a" + slug}}}
		g.used["a"+slug] = slug
	}
	g.Want("fresh", func() []TileArtist { return nil })
	g.mu.Lock()
	n, has0, used0 := len(g.tiles), len(g.tiles["g0"].photos) > 0, g.used["ag0"]
	g.mu.Unlock()
	if n > tileMax || has0 || used0 != "" {
		t.Fatalf("tiles %d, oldest kept %v, used %q", n, has0, used0)
	}
	// Busy slots are capped too.
	g.mu.Lock()
	for i := 0; i < tileMax; i++ {
		g.busy[fmt.Sprintf("b%d", i)] = true
	}
	g.mu.Unlock()
	g.Want("other", func() []TileArtist { return nil })
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy["other"] {
		t.Fatal("busy map grew past the cap")
	}
}

func TestPhotoArtistSkipsCompilations(t *testing.T) {
	for name, want := range map[string]bool{"Kendrick Lamar": true, "Various Artists": false, "Hamilton Original Cast": false, "Interstellar Soundtrack": false, "Baby Lullaby Academy": false, "": false} {
		if got := photoArtist(name); got != want {
			t.Errorf("photoArtist(%q) = %v, want %v", name, got, want)
		}
	}
}

// A picked tile survives a restart (the store is reopened), and a stored
// picture link that no longer answers HEAD is picked again.
func TestGenreTilesSurviveRestart(t *testing.T) {
	var pic bytes.Buffer
	_ = jpeg.Encode(&pic, noisy(800, 800), nil)
	dead := false
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/artist") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"name": "Some Artist", "picture_xl": srv.URL + "/pic/" + map[bool]string{false: "old", true: "new"}[dead] + ".jpg"}}})
			return
		}
		if dead && strings.HasSuffix(r.URL.Path, "old.jpg") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(pic.Bytes())
	}))
	defer srv.Close()
	path := t.TempDir() + "/genretiles.sqlite"
	store, err := OpenTileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	tiles := &GenreTiles{DeezerURL: srv.URL, HTTP: srv.Client(), Store: store}
	pickArtists := func() []TileArtist {
		return []TileArtist{{MBID: "11111111-2222-3333-4444-555555555555", Name: "Some Artist"}}
	}
	tiles.Want("rock", pickArtists)
	wait := func(g *GenreTiles) string {
		deadline := time.Now().Add(5 * time.Second)
		for g.Ready("rock") == "" && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		return g.Ready("rock")
	}
	etag := wait(tiles)
	if etag == "" {
		t.Fatal("tile not rendered")
	}
	store.Close()

	// Restart: a new store on the same file and a new GenreTiles, with no
	// network source at all.
	store, err = OpenTileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sts := store.Get("rock")
	ok := len(sts) == 1
	var st storedTile
	if ok {
		st = sts[0]
	}
	if !ok || st.MBID == "" || st.ImageURL == "" || st.PickedAt.IsZero() || len(st.JPEG) == 0 {
		t.Fatalf("stored: %v %+v", ok, st)
	}
	again := &GenreTiles{DeezerURL: "http://127.0.0.1:1", Store: store}
	if got := again.Ready("rock"); got != etag {
		t.Fatalf("after restart: ready %q, want %q", got, etag)
	}
	if b, _, ok := again.Get("rock", 0); !ok || len(b) == 0 {
		t.Fatal("no picture after restart")
	}
	// Restore also loads it into memory, and a tile dropped by the cap is
	// read back from the store.
	warm := &GenreTiles{Store: store}
	warm.Restore()
	warm.mu.Lock()
	n := len(warm.tiles)
	warm.mu.Unlock()
	if n != 1 {
		t.Fatalf("restored %d tiles", n)
	}

	// The picture link dies: the next refresh picks again.
	dead = true
	now := time.Now().Add(tileTTL + time.Hour)
	again.Now = func() time.Time { return now }
	again.DeezerURL, again.HTTP = srv.URL, srv.Client()
	again.Want("rock", pickArtists)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		again.mu.Lock()
		busy := again.busy["rock"]
		again.mu.Unlock()
		if !busy {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	old := st.ImageURL
	st = store.Get("rock")[0]
	if st.ImageURL == old || !strings.HasSuffix(st.ImageURL, "new.jpg") || st.PickedAt.Unix() < now.Unix()-1 {
		t.Fatalf("not picked again: %+v", st.ImageURL)
	}
	if b, _, ok := again.Get("rock", 0); !ok || len(b) == 0 {
		t.Fatal("art lost after a failed re-pick")
	}
}

// The soundtrack genre uses the fixed composer shortlist, with up to four
// artist photos from fanart.tv first; a Deezer picture is only a fallback
// and the genre never gets a compilation credit.
func TestSoundtrackUsesComposers(t *testing.T) {
	var pic bytes.Buffer
	_ = jpeg.Encode(&pic, noisy(800, 450), nil)
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pic.Bytes())
	}))
	defer srv.Close()
	fan := thumbArt{}
	for _, c := range scoreComposers {
		fan[strings.ToLower(c.MBID)] = srv.URL + "/" + c.MBID + ".jpg"
	}
	tiles := &GenreTiles{Fanart: fan, HTTP: srv.Client()}
	s := &Server{opts: Options{Music: variousOnly{fakeCharts{genres: []musictrend.Genre{{Slug: "soundtrack"}}}}, GenreTiles: tiles}}
	genresAnswer(t, s)
	deadline := time.Now().Add(5 * time.Second)
	for len(tiles.Versions("soundtrack")) < tilePhotos && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(tiles.Versions("soundtrack")); n != tilePhotos {
		t.Fatalf("photos %d", n)
	}
	tiles.mu.Lock()
	first := tiles.tiles["soundtrack"].photos[0].mbid
	tiles.mu.Unlock()
	if first != scoreComposers[0].MBID {
		t.Fatalf("first %s", first)
	}
	_, g := genresAnswer(t, s)
	if len(g[0].Arts) != tilePhotos || !strings.HasSuffix(strings.Split(g[0].Arts[1], "?")[0], "soundtrack.1.jpg") {
		t.Fatalf("arts %v", g[0].Arts)
	}
	w := httptest.NewRecorder()
	s.serveMusicGenreTile(w, httptest.NewRequest("GET", "/v1/music/genres/tile/soundtrack.3.jpg", nil))
	if w.Code != 200 {
		t.Fatalf("tile 3: %d", w.Code)
	}
}

type variousOnly struct{ fakeCharts }

func (variousOnly) Charts(context.Context, string, string, string, time.Time) ([]musictrend.ChartItem, bool) {
	return []musictrend.ChartItem{{Artist: "Various Artists"}}, true
}

// thumbArt answers fanart.tv artist art as a thumb per MBID.
type thumbArt map[string]string

func (f thumbArt) Art(_ context.Context, _, id string) ([]byte, string, error) {
	b, _ := json.Marshal(map[string]any{"thumb": f[id]})
	return b, "", nil
}
func (f thumbArt) Status() extras.Status { return extras.Status{State: extras.StateOK} }
