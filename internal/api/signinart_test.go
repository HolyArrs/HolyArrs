package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// signinRig is an API with Library titles (kind, rating) and fake images:
// every title has a 3840-wide textless backdrop, a poster and a logo.
func signinRig(t *testing.T, titles [][2]string) (*API, *int) {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, k := range titles {
			id := strconv.Itoa(100 + i)
			ti := domain.Title{Kind: k[0], Name: "Secret Name " + id, SortName: "secret " + id, SourceIDs: map[string]string{"tmdb": id}, Rating: k[1],
				MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
			if _, err := store.Insert(tx, "titles", &ti, domain.System); err != nil {
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
	a.signin.images = func(_ context.Context, kind, id string) (metadata.CardImages, error) {
		calls++
		b := metadata.ImageBase + "/w780/"
		en := "en"
		return metadata.CardImages{
			Backdrops: []metadata.CardImage{{Path: b + "t" + id + ".jpg", Lang: &en, Width: 3840, Height: 2160}, {Path: b + "b" + id + ".jpg", Width: 3840, Height: 2160}},
			Logos:     []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l" + id + ".png", Lang: &en, Width: 500}},
			Posters:   []metadata.CardImage{{Path: metadata.ImageBase + "/w500/p" + id + ".jpg", Width: 2000, Height: 3000}},
		}, nil
	}
	a.signin.trending = func(_ context.Context, kind string) ([]metadata.DiscoverItem, error) {
		if kind == "movie" {
			return []metadata.DiscoverItem{{Kind: "movie", TMDBID: "900", Name: "Trend A"}, {Kind: "movie", TMDBID: "901", Name: "Trend R"}}, nil
		}
		return []metadata.DiscoverItem{{Kind: "show", TMDBID: "902", Name: "Trend S"}, {Kind: "show", TMDBID: "903", Name: "Trend Unrated"}}, nil
	}
	a.signin.rating = func(_ context.Context, kind, id string) (string, error) {
		switch id {
		case "900":
			return "PG", nil
		case "901":
			return "R", nil
		case "902":
			return "TV-G", nil
		}
		return "", errors.New("no rating")
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
// The Library is used when 6 or more kid-safe titles qualify; mature and
// unrated titles never appear, and the answer holds picture addresses only.
func TestSigninArtLibraryKidSafe(t *testing.T) {
	titles := [][2]string{{"movie", "G"}, {"movie", "PG"}, {"movie", "PG-13"}, {"show", "TV-PG"}, {"show", "TV-Y7"}, {"show", "TV-G"},
		{"movie", "R"}, {"movie", "NC-17"}, {"show", "TV-14"}, {"show", "TV-MA"}, {"movie", ""}, {"movie", "NR"}}
	a, _ := signinRig(t, titles)
	w, shots := getSigninArt(t, a, "10.0.0.1")
	if w.Code != 200 {
		t.Fatalf("code %d: %s", w.Code, w.Body)
	}
	if len(shots) != 6 {
		t.Fatalf("want the 6 kid-safe titles, got %d", len(shots))
	}
	for _, s := range shots {
		id := strings.TrimSuffix(strings.TrimPrefix(s.Backdrop.URL, metadata.ImageBase+"/original/b"), ".jpg")
		n, _ := strconv.Atoi(id)
		if n < 100 || n > 105 {
			t.Fatalf("a title above the limit or unrated was used: %s", s.Backdrop.URL)
		}
		if s.Backdrop.W != 3840 || s.Poster == nil || !strings.HasSuffix(s.Logo, ".png") {
			t.Fatalf("shot: %+v", s)
		}
	}
	body := w.Body.String()
	for _, leak := range []string{"Secret", "name", "tmdb_id", "count", "rating", "\"id\""} {
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

// Fewer than 6 Library titles, or Library art turned off, uses trending
// titles, still kid-safe.
func TestSigninArtTrendingFallback(t *testing.T) {
	a, _ := signinRig(t, [][2]string{{"movie", "PG"}, {"movie", "G"}})
	_, shots := getSigninArt(t, a, "10.0.0.2")
	got := map[string]bool{}
	for _, s := range shots {
		got[s.Backdrop.URL] = true
	}
	if len(shots) != 2 || !got[metadata.ImageBase+"/original/b900.jpg"] || !got[metadata.ImageBase+"/original/b902.jpg"] {
		t.Fatalf("want trending 900 and 902 only, got %+v", shots)
	}

	many := [][2]string{{"movie", "G"}, {"movie", "PG"}, {"movie", "PG-13"}, {"show", "TV-PG"}, {"show", "TV-Y7"}, {"show", "TV-G"}}
	b, _ := signinRig(t, many)
	put := httptest.NewRequest("PUT", "/api/v1/settings/ui", strings.NewReader(`{"signin_library_art": false}`))
	pw := httptest.NewRecorder()
	b.Handler().ServeHTTP(pw, put)
	if pw.Code != 200 || b.uiSettings(context.Background()).SigninLibraryArt {
		t.Fatalf("turning Library art off: %d %s", pw.Code, pw.Body)
	}
	_, shots = getSigninArt(t, b, "10.0.0.3")
	for _, s := range shots {
		if !strings.Contains(s.Backdrop.URL, "/b90") {
			t.Fatalf("Library art shown while off: %s", s.Backdrop.URL)
		}
	}
}

// The setting defaults to on.
func TestSigninArtDefaultOn(t *testing.T) {
	a, _ := signinRig(t, nil)
	if !a.uiSettings(context.Background()).SigninLibraryArt {
		t.Fatal("Library art should default to on")
	}
}

// The answer is cached; each address gets a budget a minute.
func TestSigninArtCacheAndRateLimit(t *testing.T) {
	titles := [][2]string{{"movie", "G"}, {"movie", "PG"}, {"movie", "PG-13"}, {"show", "TV-PG"}, {"show", "TV-Y7"}, {"show", "TV-G"}}
	a, calls := signinRig(t, titles)
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a.signin.now = func() time.Time { return now }
	for i := 0; i < signinArtPerMinute; i++ {
		if w, _ := getSigninArt(t, a, "10.0.0.9"); w.Code != 200 {
			t.Fatalf("call %d: %d", i, w.Code)
		}
	}
	if *calls != 6 {
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

// A title qualifies only with a textless backdrop at least 1280 wide and a PNG logo.
func TestPickSigninShot(t *testing.T) {
	en := "en"
	b := metadata.ImageBase + "/w780/"
	logo := []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l.png", Lang: &en}}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Lang: &en, Width: 3840, Height: 2160}}, Logos: logo}); ok {
		t.Fatal("a titled backdrop qualified")
	}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Width: 1000, Height: 562}}, Logos: logo}); ok {
		t.Fatal("a small backdrop qualified")
	}
	if _, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "x.jpg", Width: 1920, Height: 1080}}}); ok {
		t.Fatal("a title without a logo qualified")
	}
	s, ok := pickSigninShot(metadata.CardImages{Backdrops: []metadata.CardImage{{Path: b + "s.jpg", Width: 1920, Height: 1080}, {Path: b + "w.jpg", Width: 3840, Height: 2160}}, Logos: logo})
	if !ok || s.Backdrop.URL != metadata.ImageBase+"/original/w.jpg" || s.Backdrop.W != 3840 {
		t.Fatalf("want the widest original: %+v", s)
	}
}
