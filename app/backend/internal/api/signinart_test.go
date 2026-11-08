package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// signinRig is an API with Library titles (kind, rating) and fake images:
// every title has a 3840-wide textless backdrop, a poster and a logo.
func signinRig(t *testing.T, titles [][2]string) (*API, *int) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, k := range titles {
			id := strconv.Itoa(100 + i)
			ti := mediafiles.Title{Kind: k[0], Name: "Secret Name " + id, SortName: "secret " + id, SourceIDs: map[string]string{"tmdb": id}, Rating: k[1],
				MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
			if _, err := database.Insert(tx, "titles", &ti, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a := New(Server{Store: s, Version: "test"})
	calls := 0
	a.signin.Images = func(_ context.Context, kind, id string) (metadata.CardImages, error) {
		calls++
		b := metadata.ImageBase + "/w780/"
		en := "en"
		return metadata.CardImages{
			Backdrops: []metadata.CardImage{{Path: b + "t" + id + ".jpg", Lang: &en, Width: 3840, Height: 2160}, {Path: b + "b" + id + ".jpg", Width: 3840, Height: 2160}},
			Logos:     []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l" + id + ".png", Lang: &en, Width: 500}},
			Posters:   []metadata.CardImage{{Path: metadata.ImageBase + "/w500/p" + id + ".jpg", Width: 2000, Height: 3000}},
		}, nil
	}
	a.signin.Trending = func(_ context.Context, kind string) ([]metadata.DiscoverItem, error) {
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
	return a, &calls
}

func getSigninArt(t *testing.T, a *API, addr string) (*httptest.ResponseRecorder, []signinShot) {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v1/public/signin-art", nil)
	r.RemoteAddr = addr + ":5555"
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	var body struct {
		Shots []signinShot `json:"shots"`
	}
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return w, body.Shots
}

// route: GET /api/v1/public/signin-art
// By default the art is trending titles (no rating check, mature titles
// included), at least 12 shots, and the answer holds picture addresses only.
func TestSigninArtTrendingDefault(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, [][2]string{{"movie", "R"}, {"show", "TV-MA"}, {"movie", ""}})
	w, shots := getSigninArt(t, a, "10.0.0.1")
	if w.Code != 200 {
		t.Fatalf("code %d: %s", w.Code, w.Body)
	}
	if len(shots) < 12 {
		t.Fatalf("want at least 12 trending shots, got %d", len(shots))
	}
	for _, s := range shots {
		if !strings.Contains(s.Backdrop.URL, "/b9") {
			t.Fatalf("Library art shown while off: %s", s.Backdrop.URL)
		}
		if s.Backdrop.W != 3840 || s.Poster == nil || !strings.HasSuffix(s.Logo, ".png") {
			t.Fatalf("shot: %+v", s)
		}
	}
	body := w.Body.String()
	for _, leak := range []string{"Secret", "Trend", "name", "tmdb_id", "count", "rating", "\"id\""} {
		if strings.Contains(body, leak) {
			t.Fatalf("answer leaks %q: %s", leak, body)
		}
	}
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &keys)
	if len(keys) != 1 {
		t.Fatalf("only shots expected: %s", body)
	}
}

// With Library art on, Library titles (any rating) come first, topped up
// with trending titles.
func TestSigninArtLibraryOn(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, [][2]string{{"movie", "R"}, {"show", "TV-MA"}})
	put := httptest.NewRequest("PUT", "/api/v1/settings/ui", strings.NewReader(`{"signin_library_art": true}`))
	pw := httptest.NewRecorder()
	a.Handler().ServeHTTP(pw, put)
	if pw.Code != 200 || !health.UISettingsOf(context.Background(), a.s.Store).SigninLibraryArt {
		t.Fatalf("turning Library art on: %d %s", pw.Code, pw.Body)
	}
	_, shots := getSigninArt(t, a, "10.0.0.3")
	lib := 0
	for _, s := range shots {
		if strings.Contains(s.Backdrop.URL, "/b10") {
			lib++
		}
	}
	if lib != 2 || len(shots) != discover.SigninArtMax {
		t.Fatalf("want 2 Library shots topped up to %d, got %d Library of %d", discover.SigninArtMax, lib, len(shots))
	}
}

// The setting defaults to off.
func TestSigninArtDefaultOff(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, nil)
	if health.UISettingsOf(context.Background(), a.s.Store).SigninLibraryArt {
		t.Fatal("Library art should default to off")
	}
}

// The answer is cached; each address gets a budget a minute.
func TestSigninArtCacheAndRateLimit(t *testing.T) {
	t.Parallel()
	a, calls := signinRig(t, nil)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a.signin.SetNow(func() time.Time { return now })
	for i := 0; i < discover.SigninArtPerMinute; i++ {
		if w, _ := getSigninArt(t, a, "10.0.0.9"); w.Code != 200 {
			t.Fatalf("call %d: %d", i, w.Code)
		}
	}
	if *calls != 16 {
		t.Fatalf("the answer was not cached: %d image calls", *calls)
	}
	w, _ := getSigninArt(t, a, "10.0.0.9")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("want 429 with Retry-After, got %d", w.Code)
	}
	if w, _ := getSigninArt(t, a, "10.0.0.10"); w.Code != 200 {
		t.Fatalf("another address was limited: %d", w.Code)
	}
	now = now.Add(61 * time.Second)
	if w, _ := getSigninArt(t, a, "10.0.0.9"); w.Code != 200 {
		t.Fatalf("budget did not refill: %d", w.Code)
	}
}

type signinShot = discover.SigninShot

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
