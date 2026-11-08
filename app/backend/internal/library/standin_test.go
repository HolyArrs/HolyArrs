package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func jpegOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func fill(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// photo is a picture with the spread of a real photograph: noise over a
// gradient (the same bytes for the same seed).
func photo(w, h int, seed int64) *image.RGBA {
	r := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), uint8(r.Intn(256)), 255})
		}
	}
	return img
}

func greyPerson(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "grey-person-outline.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestStandInReason: the stand-in rules. A known stand-in, one flat colour,
// a flat picture with only a faint outline (the grey person) and a picture
// one colour fills nearly all of are stand-ins; photos, flat-looking but
// high-contrast artwork and files that are not images are not.
func TestStandInReason(t *testing.T) {
	// A faint outline on a light flat field: not in the known list, caught by its spread alone.
	outline := fill(400, 400, color.RGBA{233, 231, 238, 255})
	for y := 100; y < 300; y++ {
		for x := 150; x < 250; x++ {
			if (x-200)*(x-200)+(y-200)*(y-200) < 80*80 && (x-200)*(x-200)+(y-200)*(y-200) > 55*55 {
				outline.Set(x, y, color.RGBA{193, 192, 198, 255})
			}
		}
	}
	// 4% of the picture is a mark, the rest one colour.
	mostly := fill(400, 400, color.RGBA{10, 10, 10, 255})
	for y := 0; y < 400; y++ {
		for x := 0; x < 25; x++ {
			mostly.Set(x, y, color.RGBA{250, 250, 250, 255})
		}
	}
	// A black and white graphic like an album cover: a quarter white, three quarters black.
	cover := fill(400, 400, color.RGBA{0, 0, 0, 255})
	for y := 0; y < 400; y++ {
		for x := 0; x < 100; x++ {
			cover.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	// A picture that is clear all over (a transparent PNG).
	var clear bytes.Buffer
	if err := png.Encode(&clear, image.NewNRGBA(image.Rect(0, 0, 200, 200))); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		b     []byte
		stand bool
	}{
		{"the grey person from the old source", greyPerson(t), true},
		{"the metadata server's grey stand-in", mustRead(t, filepath.Join("testdata", "server-placeholder.jpg")), true},
		{"solid black", jpegOf(t, fill(1000, 1000, color.Black)), true},
		{"solid white", jpegOf(t, fill(500, 500, color.White)), true},
		{"solid colour", jpegOf(t, fill(300, 300, color.RGBA{30, 120, 200, 255})), true},
		{"faint outline on a flat field", jpegOf(t, outline), true},
		{"one colour fills 94 percent", jpegOf(t, mostly), false},
		{"clear PNG", clear.Bytes(), true},
		{"a photo", jpegOf(t, photo(300, 300, 1)), false},
		{"black and white artwork", jpegOf(t, cover), false},
		{"not an image", []byte("\xff\xd8\xff\xe0 not really a picture"), false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		if got := StandInReason(c.b); (got != "") != c.stand {
			t.Errorf("%s: reason %q, stand-in want %v", c.name, got, c.stand)
		}
	}
	// 96 percent one colour is nearly all of it.
	nearly := fill(400, 400, color.RGBA{10, 10, 10, 255})
	for y := 0; y < 400; y++ {
		for x := 0; x < 12; x++ {
			nearly.Set(x, y, color.RGBA{250, 250, 250, 255})
		}
	}
	if StandInReason(jpegOf(t, nearly)) == "" {
		t.Error("a picture one colour fills 97 percent of is a stand-in")
	}
	// The known list holds the grey person's sum, so it needs no rule.
	sum := sha256.Sum256(greyPerson(t))
	if !serverPlaceholders[hex.EncodeToString(sum[:])] {
		t.Error("the grey person is not in the known list")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestArtistPhotoStandInNeverSaved: the cache keeps no stand-in as an
// artist's photo, whatever the address answers, and still keeps a real one.
// Pictures that are not an artist's photo (a logo is mostly one colour) are
// not judged.
func TestArtistPhotoStandInNeverSaved(t *testing.T) {
	bodies := map[string][]byte{
		"/v1/img/artist/grey/500.webp":  greyPerson(t),
		"/v1/img/artist/black/500.webp": jpegOf(t, fill(1000, 1000, color.Black)),
		"/v1/img/artist/real/500.webp":  jpegOf(t, photo(200, 200, 2)),
		"/v1/img/logo/clear/1200.png":   jpegOf(t, fill(100, 100, color.Black)),
	}
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(b)
	}))
	defer pics.Close()
	dir := t.TempDir()
	c := &ArtworkCache{Dir: dir, Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	for _, p := range []string{"grey", "black"} {
		if _, err := c.FetchStored(context.Background(), pics.URL+"/v1/img/artist/"+p+"/500.webp", nil); !errors.Is(err, ErrArtworkRejected) {
			t.Errorf("%s stand-in stored: %v", p, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ArtworkName(pics.URL+"/v1/img/artist/"+p+"/500.webp"))); !os.IsNotExist(err) {
			t.Errorf("%s stand-in left in the cache", p)
		}
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("cache holds %d files after two refusals", len(es))
	}
	if _, err := c.FetchStored(context.Background(), pics.URL+"/v1/img/artist/real/500.webp", nil); err != nil {
		t.Errorf("a real photo was refused: %v", err)
	}
	if _, err := c.FetchStored(context.Background(), pics.URL+"/v1/img/logo/clear/1200.png", nil); err != nil {
		t.Errorf("a logo was judged as a photo: %v", err)
	}
}

// standInWorld is a fake metadata server whose artist photo answer is set
// per artist: a real photo, a stand-in (the grey person), nothing, or an error.
type standInWorld struct {
	srv  *httptest.Server
	mu   sync.Mutex
	mode map[string]string
	asks map[string]int
	real []byte
}

func newStandInWorld(t *testing.T) *standInWorld {
	w := &standInWorld{mode: map[string]string{}, asks: map[string]int{}, real: jpegOf(t, photo(160, 160, 3))}
	grey := greyPerson(t)
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/") // v1 img artist <mbid> 500.webp
		if len(parts) != 5 || parts[2] != "artist" {
			http.NotFound(rw, r)
			return
		}
		w.mu.Lock()
		mode := w.mode[parts[3]]
		w.asks[parts[3]]++
		w.mu.Unlock()
		switch mode {
		case "real":
			rw.Header().Set("Content-Type", "image/jpeg")
			_, _ = rw.Write(w.real)
		case "standin":
			rw.Header().Set("Content-Type", "image/jpeg")
			_, _ = rw.Write(grey)
		case "error":
			http.Error(rw, "trouble", http.StatusInternalServerError)
		default:
			http.NotFound(rw, r)
		}
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *standInWorld) set(mbid, mode string) {
	w.mu.Lock()
	w.mode[mbid] = mode
	w.mu.Unlock()
}

func (w *standInWorld) asked(mbid string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.asks[mbid]
}

// addStandInArtist adds another artist to the lab MusicBrainz and the Library.
func addStandInArtist(t *testing.T, svc *Service, fm *fakeMusic, mbid, name string) int64 {
	t.Helper()
	fm.artists[mbid] = metadata.Artist{MBID: mbid, Name: name, Type: "person"}
	id, _, err := svc.AddArtist(context.Background(), mbid, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func setArtwork(t *testing.T, svc *Service, id int64, entries ...string) {
	t.Helper()
	list := `["` + strings.Join(entries, `","`) + `"]`
	if len(entries) == 0 {
		list = "[]"
	}
	err := svc.Store.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET artwork = ? WHERE id = ?", list, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// keepFile puts bytes in the cache under a name made from key and answers the artwork entry for it.
func keepFile(t *testing.T, svc *Service, key string, b []byte) string {
	t.Helper()
	if err := os.MkdirAll(svc.Artwork.Dir, 0o750); err != nil {
		t.Fatal(err)
	}
	name := ArtworkName("https://old.test/" + key + ".jpg")
	if err := os.WriteFile(filepath.Join(svc.Artwork.Dir, name), b, 0o640); err != nil {
		t.Fatal(err)
	}
	return "poster:" + name
}

const (
	mbReplace = "a0000000-0000-0000-0000-0000000000a1" // stand-in, the server has a real photo
	mbNone    = "a0000000-0000-0000-0000-0000000000a2" // solid black, the server has no photo
	mbReal    = "a0000000-0000-0000-0000-0000000000a3" // a real photo: never touched
	mbTrim    = "a0000000-0000-0000-0000-0000000000a4" // a stand-in beside a real picture
	mbDown    = "a0000000-0000-0000-0000-0000000000a5" // stand-in, the server is in trouble
	mbGrey    = "a0000000-0000-0000-0000-0000000000a6" // stand-in, and the server answers with a stand-in too
)

// TestRepairArtistStandIns: the one-time repair gives an artist whose stored
// picture is only a stand-in the server's real photo, leaves the artwork
// empty when the server has none (so the screen's own fallback shows), keeps
// real pictures, and runs once; the ordinary picture pass fills an emptied
// artist later when the server comes back.
func TestRepairArtistStandIns(t *testing.T) {
	ctx := context.Background()
	svc, s, fm, _ := musicService(t)
	w := newStandInWorld(t)
	svc.ImageServer = w.srv.URL + "/v1/img"
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{w.srv.URL + "/"}, HTTP: &http.Client{}}
	for k, v := range map[string]string{mbReplace: "real", mbNone: "none", mbReal: "real", mbTrim: "real", mbDown: "error", mbGrey: "standin"} {
		w.set(k, v)
	}
	grey := keepFile(t, svc, "grey", greyPerson(t))
	black := keepFile(t, svc, "black", jpegOf(t, fill(1000, 1000, color.Black)))
	realOne := keepFile(t, svc, "real", jpegOf(t, photo(120, 120, 9)))
	ids := map[string]int64{}
	for mbid, name := range map[string]string{mbReplace: "Coldplay", mbNone: "Ye", mbReal: "Adele", mbTrim: "Queen", mbDown: "Radiohead", mbGrey: "Pink Outline"} {
		ids[mbid] = addStandInArtist(t, svc, fm, mbid, name)
	}
	setArtwork(t, svc, ids[mbReplace], grey)
	setArtwork(t, svc, ids[mbNone], black)
	setArtwork(t, svc, ids[mbReal], realOne)
	setArtwork(t, svc, ids[mbTrim], grey, realOne)
	setArtwork(t, svc, ids[mbDown], grey)
	setArtwork(t, svc, ids[mbGrey], grey)
	asked := w.asked(mbReal)

	res, err := svc.RepairArtistStandIns(ctx, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(res.Emptied)
	if res.Looked != 6 || !slices.Equal(res.Replaced, []string{"Coldplay"}) || !slices.Equal(res.Emptied, []string{"Pink Outline", "Radiohead", "Ye"}) || !slices.Equal(res.Trimmed, []string{"Queen"}) || res.Fixed() != 5 {
		t.Fatalf("repair %+v", res)
	}
	art := func(mbid string) []string { return titleOf(t, s, ids[mbid]).Artwork }
	if a := art(mbReplace); len(a) != 1 || a[0] == grey || !strings.HasPrefix(a[0], "poster:") || !strings.HasSuffix(a[0], ".webp") {
		t.Errorf("replaced artwork %v", a)
	}
	if _, ok := svc.Artwork.Path(strings.TrimPrefix(art(mbReplace)[0], "poster:")); !ok {
		t.Error("the server's photo is not in the cache")
	}
	for _, mbid := range []string{mbNone, mbDown, mbGrey} {
		if a := art(mbid); len(a) != 0 {
			t.Errorf("%s: artwork %v, want empty", mbid, a)
		}
	}
	if a := art(mbReal); len(a) != 1 || a[0] != realOne {
		t.Errorf("a real picture was changed: %v", a)
	}
	if a := art(mbTrim); len(a) != 1 || a[0] != realOne {
		t.Errorf("trimmed artwork %v", a)
	}
	if w.asked(mbReal) != asked {
		t.Error("the server was asked about an artist with a real picture")
	}

	// Once: a stand-in put back now is left alone by a second run.
	setArtwork(t, svc, ids[mbReplace], grey)
	again, err := svc.RepairArtistStandIns(ctx, domain.System)
	if err != nil || again.Looked != 0 || again.Fixed() != 0 {
		t.Fatalf("second run %+v %v", again, err)
	}
	setArtwork(t, svc, ids[mbReplace])

	// The server comes back: the ordinary picture pass fills the emptied artist.
	w.set(mbDown, "real")
	out, err := svc.FillAllArtistPictures(ctx, domain.System, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a := art(mbDown); len(a) != 1 || !strings.HasPrefix(a[0], "poster:") {
		t.Errorf("emptied artist not filled later: %v (result %+v)", a, out)
	}
	if a := art(mbNone); len(a) != 0 {
		t.Errorf("an artist the server has no photo of got %v", a)
	}
}

// TestRefreshReplacesStandIn: the ordinary artist refresh (and the single
// picture Job) swaps a stand-in saved earlier for the server's photo, so
// nothing waits for the one-time repair; real pictures stay.
func TestRefreshReplacesStandIn(t *testing.T) {
	ctx := context.Background()
	svc, s, _, _ := musicService(t)
	w := newStandInWorld(t)
	w.set(labArtist, "real")
	svc.ImageServer = w.srv.URL + "/v1/img"
	svc.ArtistImages = &metadata.ArtistImages{BaseURL: w.srv.URL, Gap: time.Millisecond, Timeout: time.Second}
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{w.srv.URL + "/"}, HTTP: &http.Client{}}
	id := addBareArtist(t, svc)
	grey := keepFile(t, svc, "grey", greyPerson(t))
	setArtwork(t, svc, id, grey)

	src, err := svc.FillArtistPictures(ctx, id, domain.System)
	if err != nil || src != SourceServer {
		t.Fatalf("picture Job: %q %v", src, err)
	}
	if a := titleOf(t, s, id).Artwork; len(a) != 1 || a[0] == grey || !strings.HasSuffix(a[0], ".webp") {
		t.Fatalf("artwork after the picture Job %v", a)
	}

	// The refresh path: stand-in again, real photo answered, no repair involved.
	setArtwork(t, svc, id, grey)
	if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if a := titleOf(t, s, id).Artwork; len(a) != 1 || a[0] == grey || !strings.HasSuffix(a[0], ".webp") {
		t.Fatalf("artwork after the refresh %v", a)
	}
	// A real picture is never replaced by a refresh.
	own := keepFile(t, svc, "real", jpegOf(t, photo(120, 120, 5)))
	setArtwork(t, svc, id, own)
	asked := w.asked(labArtist)
	if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if a := titleOf(t, s, id).Artwork; len(a) != 1 || a[0] != own || w.asked(labArtist) != asked {
		t.Fatalf("a real picture was touched: %v, asked %d times", a, w.asked(labArtist)-asked)
	}
}

// A picture claiming a huge size is never decoded, only left unjudged.
func TestStandInReasonSkipsHugePictures(t *testing.T) {
	var buf bytes.Buffer
	// A PNG header that claims 100000 × 100000 pixels.
	buf.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR\x00\x01\x86\xa0\x00\x01\x86\xa0\x08\x02\x00\x00\x00"))
	buf.Write(make([]byte, 4))
	if r := StandInReason(buf.Bytes()); r != "" {
		t.Fatalf("huge picture judged: %q", r)
	}
}
