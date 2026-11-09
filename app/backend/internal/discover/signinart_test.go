package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A title qualifies only with a textless backdrop at least 1920 wide and a PNG logo.
func TestPickSigninShot(t *testing.T) {
	t.Parallel()
	en := "en"
	b := metadata.ImageBase + "/w780/"
	logo := []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l.png", Lang: &en}}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Lang: &en, Width: 3840, Height: 2160}}, Logos: logo}); ok {
		t.Fatal("a titled backdrop qualified")
	}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Width: 1280, Height: 720}}, Logos: logo}); ok {
		t.Fatal("a small backdrop qualified")
	}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Width: 1920, Height: 1080}}}); ok {
		t.Fatal("a title without a logo qualified")
	}
	s, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "s.jpg", Width: 1920, Height: 1080}, {Path: b + "w.jpg", Width: 3840, Height: 2160}}, Logos: logo})
	if !ok || s.Backdrop.URL != metadata.ImageBase+"/original/w.jpg" || s.Backdrop.W != 3840 {
		t.Fatalf("want the widest original: %+v", s)
	}
	s, _ = pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "sq.jpg", Width: 3840, Height: 2400}, {Path: b + "hd.jpg", Width: 2560, Height: 1440}}, Logos: logo})
	if s.Backdrop.URL != metadata.ImageBase+"/original/hd.jpg" {
		t.Fatalf("want the 16:9 one: %+v", s)
	}
}

// signinRig is a Signin with Library titles (kind, rating) and fake images:
// every title has a 3840-wide textless backdrop, a poster and a logo.
func signinRig(t *testing.T, titles [][2]string) (*Signin, *int) {
	t.Helper()
	a := testSignin(t)
	s := a.Store
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
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
	a.ServeArtwork = library.ServeArtwork
	a.LibraryArtOn = func(ctx context.Context) bool { return health.SigninLibraryArtOn(ctx, s) }
	calls := 0
	images := a.Images
	a.Images = func(ctx context.Context, kind, id string) (metadata.CardImages, error) {
		calls++
		return images(ctx, kind, id)
	}
	return a, &calls
}

// signinMux serves the sign-in routes as api registers them.
func signinMux(a *Signin) *http.ServeMux {
	m := http.NewServeMux()
	a.Routes(m)
	return m
}

func getSigninArt(t *testing.T, a *Signin, addr string) (*httptest.ResponseRecorder, []SigninShot) {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v1/public/signin-art", nil)
	r.RemoteAddr = addr + ":5555"
	w := httptest.NewRecorder()
	signinMux(a).ServeHTTP(w, r)
	var body struct {
		Shots []SigninShot `json:"shots"`
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
	err := a.Store.Write(context.Background(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", health.SettingSigninLibraryArt, "on", domain.System, time.Now())
	})
	if err != nil || !health.UISettingsOf(context.Background(), a.Store).SigninLibraryArt {
		t.Fatalf("turning Library art on: %v", err)
	}
	_, shots := getSigninArt(t, a, "10.0.0.3")
	lib := 0
	for _, s := range shots {
		if strings.Contains(s.Backdrop.URL, "/b10") {
			lib++
		}
	}
	if lib != 2 || len(shots) != SigninArtMax {
		t.Fatalf("want 2 Library shots topped up to %d, got %d Library of %d", SigninArtMax, lib, len(shots))
	}
}

// The setting defaults to off.
func TestSigninArtDefaultOff(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, nil)
	if health.UISettingsOf(context.Background(), a.Store).SigninLibraryArt {
		t.Fatal("Library art should default to off")
	}
}

// The answer is cached; each address gets a budget a minute.
func TestSigninArtCacheAndRateLimit(t *testing.T) {
	t.Parallel()
	a, calls := signinRig(t, nil)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a.SetNow(func() time.Time { return now })
	for i := 0; i < SigninArtPerMinute; i++ {
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
