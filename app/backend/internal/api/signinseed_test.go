package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// /login carries the art list and tiny thumbnails of the first slide, built
// from TMDB's small size only, capped, and nothing else.
func TestSigninSeedInlinesFirstSlideThumbnails(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, nil)
	var mu sync.Mutex
	var asked []string
	pic := testJPEG(t, 300, 169)
	a.signinSeed.fetch = func(_ context.Context, url string) ([]byte, error) {
		mu.Lock()
		asked = append(asked, url)
		mu.Unlock()
		return pic, nil
	}
	if a.signinSeedJSON(context.Background()) != nil {
		t.Fatal("seed before the art is built")
	}
	shots := a.signinShots(context.Background())
	if len(shots) == 0 {
		t.Fatal("no shots")
	}
	var got []byte
	for i := 0; i < 200 && !bytes.Contains(got, []byte(`"poster"`)); i++ {
		got = a.signinSeedJSON(context.Background())
		time.Sleep(5 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("thumbnails never ready")
	}
	var body struct {
		Shots []signinShot                 `json:"shots"`
		Seed  map[string]map[string]string `json:"seed"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Shots) != len(shots) || body.Shots[0].Backdrop.URL != publicShots(shots)[0].Backdrop.URL {
		t.Fatalf("shots %+v", body.Shots)
	}
	for _, k := range []string{"backdrop", "poster"} {
		for _, s := range []string{"t24", "t64"} {
			v := body.Seed[k][s]
			if !strings.HasPrefix(v, "data:image/jpeg;base64,") || len(v) > signinSeedThumbMax {
				t.Fatalf("%s %s: %d bytes", k, s, len(v))
			}
		}
	}
	if strings.Contains(string(got), "Secret") || strings.Contains(string(got), "Trend") {
		t.Fatal("names leaked")
	}
	mu.Lock()
	for _, u := range asked {
		if !strings.HasPrefix(u, metadata.ImageBase+"/w300/") && !strings.HasPrefix(u, metadata.ImageBase+"/w342/") {
			t.Fatalf("fetched %s", u)
		}
	}
	mu.Unlock()
}

func TestSigninThumbRefusesOthers(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, nil)
	a.signinSeed.fetch = func(context.Context, string) ([]byte, error) { return nil, errors.New("must not fetch") }
	if _, err := a.makeSigninThumb("https://evil.example/original/x.jpg", 300); err == nil {
		t.Fatal("non-TMDB address accepted")
	}
	if _, err := a.makeSigninThumb(metadata.ImageBase+"/original/../x.jpg", 300); err == nil {
		t.Fatal("path accepted")
	}
	if _, err := signinThumbOf(testJPEG(t, 4000, 100)); err == nil {
		t.Fatal("huge picture accepted")
	}
	if _, err := signinThumbOf([]byte("not a jpeg")); err == nil {
		t.Fatal("junk accepted")
	}
}
