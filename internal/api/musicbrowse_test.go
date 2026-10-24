package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// tplRig is a music_lists_v1 rig whose metadata server answers the catalog
// bundle from testdata (real data, trimmed to a few keys).
func tplRig(t *testing.T) *askRig {
	t.Helper()
	r := listRig(t, "household_empty.json")
	body, err := os.ReadFile("testdata/music_pools_small.json")
	if err != nil {
		t.Fatal(err)
	}
	r.api.s.MusicPools = func(_ context.Context, etag string) ([]byte, string, error) { return body, `"fixture"`, nil }
	return r
}

type tplAnswer struct {
	Kind  string      `json:"kind"`
	Tile  MusicCard   `json:"tile"`
	Hero  *MusicCard  `json:"hero"`
	Alts  []MusicCard `json:"hero_alts"`
	Rows  []MusicRow  `json:"rows"`
	Genre string      `json:"genre"`
}

func checkTplRows(t *testing.T, a tplAnswer, min int) {
	t.Helper()
	if len(a.Rows) < min {
		t.Fatalf("%s: %d rows, want at least %d", a.Genre, len(a.Rows), min)
	}
	seen := map[string]bool{}
	if a.Hero != nil {
		seen[a.Hero.Key] = true
	}
	for _, c := range a.Alts {
		seen[c.Key] = true
	}
	for _, row := range a.Rows {
		if len(row.Items) < 8 {
			t.Fatalf("%s: row %s has %d items", a.Genre, row.ID, len(row.Items))
		}
		if strings.TrimSpace(row.Reason) == "" || strings.Contains(row.Title, "{") {
			t.Fatalf("%s: row %s text %q / %q", a.Genre, row.ID, row.Title, row.Reason)
		}
		for _, c := range row.Items {
			if c.Kind == "album" {
				if seen[c.Key] {
					t.Fatalf("%s: album %s twice (row %s)", a.Genre, c.Name, row.ID)
				}
				seen[c.Key] = true
			}
		}
	}
}

// route: GET /api/v1/music/genre/{slug} (music_lists_v1, row templates)
func TestTplGenrePage(t *testing.T) {
	r := tplRig(t)
	var a tplAnswer
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/music/genre/rock?region=US", "", 200), &a); err != nil {
		t.Fatal(err)
	}
	checkTplRows(t, a, 12)
	if a.Tile.Name != "Rock" || a.Hero == nil || len(a.Alts) != 2 {
		t.Fatalf("tile %q hero %v alts %d", a.Tile.Name, a.Hero, len(a.Alts))
	}
	// An old chart slug reaches the same page.
	var b tplAnswer
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/music/genre/hard-rock?region=US", "", 200), &b)
	if b.Tile.Name != "Rock" {
		t.Fatalf("hard-rock alias tile = %q", b.Tile.Name)
	}
}

// route: GET /api/v1/music/browse/{kind}/{slug}
func TestTplBrowsePages(t *testing.T) {
	r := tplRig(t)
	for _, c := range []struct {
		path string
		min  int
	}{{"/api/v1/music/browse/mood/chill?region=US", 6}, {"/api/v1/music/browse/decade/90s?region=US", 6}, {"/api/v1/music/browse/activity/driving?region=US", 6}, {"/api/v1/music/browse/season/halloween?region=US", 1}} {
		var a tplAnswer
		if err := json.Unmarshal(r.owner.call("GET", c.path, "", 200), &a); err != nil {
			t.Fatal(err)
		}
		a.Genre = c.path
		checkTplRows(t, a, c.min)
	}
	r.owner.call("GET", "/api/v1/music/browse/mood/nope?region=US", "", 404)
	r.owner.call("GET", "/api/v1/music/browse/planet/x?region=US", "", 400)
}

// route: GET /api/v1/music/browse-tiles
func TestTplBrowseTiles(t *testing.T) {
	r := tplRig(t)
	var a struct {
		Sections []struct {
			ID    string `json:"id"`
			Tiles []struct {
				Key    string   `json:"key"`
				Covers []string `json:"covers"`
			} `json:"tiles"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/music/browse-tiles?region=us", "", 200), &a); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, s := range a.Sections {
		n += len(s.Tiles)
	}
	if len(a.Sections) != 5 || n != 61 {
		t.Fatalf("%d sections, %d tiles", len(a.Sections), n)
	}
}

// The kill switch keeps today's pages.
func TestTplKillSwitch(t *testing.T) {
	r := tplRig(t)
	r.api.s.MusicRowsTplOff = true
	r.owner.call("GET", "/api/v1/music/browse/mood/chill?region=US", "", 404)
	r.owner.call("GET", "/api/v1/music/browse-tiles", "", 404)
	resp, err := r.owner.c.Get(r.owner.base + "/api/v1/music/genre/rock?region=US")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(b), `"tpl:`) {
		t.Fatal("engine rows with the kill switch on")
	}
}

// Kid accounts get nothing from any engine music page (music has no age
// ratings), before anything is read.
func TestTplKidGetsNothing(t *testing.T) {
	r := tplRig(t)
	reads := 0
	r.api.s.MusicPools = func(context.Context, string) ([]byte, string, error) { reads++; return nil, "", errors.New("must not be read") }
	kid := auth.Who{UserID: 7, Role: auth.RoleMember, MaxRating: "PG"}
	for _, c := range []struct {
		path string
		vals map[string]string
		h    func(http.ResponseWriter, *http.Request)
	}{
		{"/api/v1/music/browse/mood/chill", map[string]string{"kind": "mood", "slug": "chill"}, r.api.musicBrowsePage},
		{"/api/v1/music/browse/season/halloween", map[string]string{"kind": "season", "slug": "halloween"}, r.api.musicBrowsePage},
		{"/api/v1/music/browse/decade/90s", map[string]string{"kind": "decade", "slug": "90s"}, r.api.musicBrowsePage},
		{"/api/v1/music/browse/activity/driving", map[string]string{"kind": "activity", "slug": "driving"}, r.api.musicBrowsePage},
		{"/api/v1/music/browse-tiles", nil, r.api.musicBrowseTiles},
		{"/api/v1/music/genre/rock", map[string]string{"slug": "rock"}, r.api.musicGenrePage},
	} {
		req := httptest.NewRequest("GET", c.path+"?region=US", nil)
		for k, v := range c.vals {
			req.SetPathValue(k, v)
		}
		req = req.WithContext(auth.ContextWith(req.Context(), kid))
		w := httptest.NewRecorder()
		c.h(w, req)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, `"kid":true`) || strings.Contains(body, `"tpl:`) || strings.Contains(body, `"covers"`) {
			t.Fatalf("%s: %d %s", c.path, w.Code, body)
		}
	}
	if reads != 0 {
		t.Fatalf("the bundle was read %d times for a Kid", reads)
	}
}

// One download at a time; a failure is kept for minutes, so a burst of
// page views during a failure costs one download.
func TestTplBundleSingleFlightAndFailureCache(t *testing.T) {
	r := tplRig(t)
	var calls int32
	r.api.s.MusicPools = func(context.Context, string) ([]byte, string, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(30 * time.Millisecond)
		return nil, "", metadata.ErrServiceUnavailable
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = r.api.musicBundle(context.Background(), time.Now()) }()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("%d downloads during a failure, want 1", n)
	}
	// A bad bundle with a good copy kept: the copy keeps serving and is
	// cached again (no download per request).
	r2 := tplRig(t)
	good := r2.api.musicBundle(context.Background(), time.Now())
	if good == nil {
		t.Fatal("no bundle from the fixture")
	}
	later := time.Now().Add(7 * time.Hour)
	calls = 0
	r2.api.s.MusicPools = func(context.Context, string) ([]byte, string, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("{not json"), `"bad"`, nil
	}
	for i := 0; i < 5; i++ {
		if b := r2.api.musicBundle(context.Background(), later); b != good {
			t.Fatal("the last good bundle did not keep serving")
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("%d downloads of a bad bundle, want 1", n)
	}
}

// A cancelled request neither cancels the download nor leaves a fail mark.
func TestTplCancelledRequestLeavesNoFailMark(t *testing.T) {
	r := tplRig(t)
	body, _ := os.ReadFile("testdata/music_pools_small.json")
	var sawCancelled bool
	fail := true
	r.api.s.MusicPools = func(ctx context.Context, _ string) ([]byte, string, error) {
		sawCancelled = sawCancelled || ctx.Err() != nil
		if fail {
			return nil, "", context.Canceled
		}
		return body, `"fixture"`, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b := r.api.musicBundle(ctx, time.Now()); b != nil {
		t.Fatal("bundle from a failed download")
	}
	if sawCancelled {
		t.Fatal("the download used the cancelled request context")
	}
	if _, marked := r.api.musicv2.cand.Get("tpl:fail", time.Now()); marked {
		t.Fatal("a cancelled request left a fail mark")
	}
	fail = false
	if b := r.api.musicBundle(context.Background(), time.Now()); b == nil {
		t.Fatal("the next request did not download")
	}
}
