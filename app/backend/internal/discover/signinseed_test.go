package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// testSignin is a Signin over an empty database with fake images and a
// fake trending list (twenty titles of each kind).
func testSignin(t *testing.T) *Signin {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &Signin{Store: s}
	a.Images = func(_ context.Context, kind, id string) (metadata.CardImages, error) {
		b := metadata.ImageBase + "/w780/"
		en := "en"
		return metadata.CardImages{
			Backdrops: []metadata.CardImage{{Path: b + "t" + id + ".jpg", Lang: &en, Width: 3840, Height: 2160}, {Path: b + "b" + id + ".jpg", Width: 3840, Height: 2160}},
			Logos:     []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l" + id + ".png", Lang: &en, Width: 500}},
			Posters:   []metadata.CardImage{{Path: metadata.ImageBase + "/w500/p" + id + ".jpg", Width: 2000, Height: 3000}},
		}, nil
	}
	a.Trending = func(_ context.Context, kind string) ([]metadata.DiscoverItem, error) {
		var out []metadata.DiscoverItem
		for i := 0; i < 8; i++ {
			id := strconv.Itoa(900 + i)
			if kind == "show" {
				id = strconv.Itoa(950 + i)
			}
			out = append(out, metadata.DiscoverItem{Kind: kind, TMDBID: id, Name: "Trend " + id, Popularity: float64(100 - i)})
		}
		return out, nil
	}
	return a
}

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
	a := testSignin(t)
	var mu sync.Mutex
	var asked []string
	pic := testJPEG(t, 300, 169)
	a.Fetch = func(_ context.Context, url string) ([]byte, error) {
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
	// Wait for the poster thumbnail itself: "poster" also names a field of
	// every shot, so finding the word alone can end the wait while only the
	// backdrop is built (seen on slower CI machines).
	var got []byte
	for i := 0; i < 400 && !seedHasPoster(got); i++ {
		got = a.signinSeedJSON(context.Background())
		time.Sleep(5 * time.Millisecond)
	}
	if got == nil {
		t.Fatal("thumbnails never ready")
	}
	var body struct {
		Shots []SigninShot                 `json:"shots"`
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
	a := testSignin(t)
	a.Fetch = func(context.Context, string) ([]byte, error) { return nil, errors.New("must not fetch") }
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

// A flood of /login opens while the sign-in art is stale starts one
// background build, not one parked goroutine per request.
func TestSigninSeedFloodStartsOneBuild(t *testing.T) {
	a := testSignin(t)
	release := a.HoldBuild()
	base := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		a.signinSeedJSON(context.Background())
	}
	time.Sleep(20 * time.Millisecond)
	grew := runtime.NumGoroutine() - base
	release()
	if grew > 5 {
		t.Fatalf("%d goroutines started for 100 /login opens", grew)
	}
}

// seedHasPoster reports whether a /login seed carries the poster thumbnails.
func seedHasPoster(b []byte) bool {
	var body struct {
		Seed map[string]json.RawMessage `json:"seed"`
	}
	return b != nil && json.Unmarshal(b, &body) == nil && body.Seed["poster"] != nil
}
