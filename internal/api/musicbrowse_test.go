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
	Thin    bool `json:"thin"`
	Similar []struct {
		Key  string `json:"key"`
		Href string `json:"href"`
	} `json:"similar"`
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
	thin := 0
	for _, path := range []string{"/api/v1/music/browse/mood/chill?region=US", "/api/v1/music/browse/decade/90s?region=US", "/api/v1/music/browse/activity/driving?region=US", "/api/v1/music/browse/season/halloween?region=US"} {
		var a tplAnswer
		if err := json.Unmarshal(r.owner.call("GET", path, "", 200), &a); err != nil {
			t.Fatal(err)
		}
		a.Genre = path
		// A page shows 12 rows or more, or none: a short note with similar
		// pages instead of a near-empty page.
		if a.Thin {
			thin++
			if len(a.Rows) != 0 || a.Similar == nil {
				t.Fatalf("%s: thin page with %d rows, similar %v", path, len(a.Rows), a.Similar)
			}
			continue
		}
		checkTplRows(t, a, 12)
	}
	if thin == 0 {
		t.Fatal("the small fixture should leave at least one page thin")
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
	for _, sec := range a.Sections {
		for _, tl := range sec.Tiles {
			n++
			// Every shown tile opens a page with enough rows.
			kind, slug, _ := strings.Cut(tl.Key, ":")
			if kind == "decade" {
				slug = slug[2:]
			}
			if tl.Key == "activity:party" {
				kind, slug = "mood", "party"
			}
			path := "/api/v1/music/browse/" + kind + "/" + slug + "?region=us"
			if kind == "genre" {
				path = "/api/v1/music/genre/" + slug + "?region=us"
			}
			var p tplAnswer
			if err := json.Unmarshal(r.owner.call("GET", path, "", 200), &p); err != nil || p.Thin || len(p.Rows) < 12 {
				t.Fatalf("shown tile %s opens a page with %d rows (thin %v)", tl.Key, len(p.Rows), p.Thin)
			}
		}
	}
	if n == 0 || n == 61 {
		t.Fatalf("%d tiles shown: want only the pages with enough rows", n)
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

// A cancelled caller never cancels the download; during a real failure it
// still leaves the fail mark, so cancel-and-retry costs one download.
func TestTplCancelledCallersDuringAFailure(t *testing.T) {
	r := tplRig(t)
	var calls int32
	var sawCancelled atomic.Bool
	r.api.s.MusicPools = func(ctx context.Context, _ string) ([]byte, string, error) {
		atomic.AddInt32(&calls, 1)
		if ctx.Err() != nil {
			sawCancelled.Store(true)
		}
		return nil, "", metadata.ErrServiceUnavailable
	}
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = r.api.musicBundle(ctx, time.Now())
		_ = r.api.musicBundle(context.Background(), time.Now())
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("%d downloads for 20 cancel-and-retry requests, want 1", n)
	}
	if sawCancelled.Load() {
		t.Fatal("the download used a cancelled request context")
	}
	if _, marked := r.api.musicv2.cand.Get("tpl:fail", time.Now()); !marked {
		t.Fatal("no fail mark after a real failure")
	}
}

// A request waiting for the download gives up when its own context ends.
func TestTplWaiterRespectsItsContext(t *testing.T) {
	r := tplRig(t)
	release := make(chan struct{})
	r.api.s.MusicPools = func(context.Context, string) ([]byte, string, error) {
		<-release
		return nil, "", metadata.ErrServiceUnavailable
	}
	go func() { _ = r.api.musicBundle(context.Background(), time.Now()) }()
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if b := r.api.musicBundle(ctx, time.Now()); b != nil || time.Since(start) > time.Second {
		t.Fatalf("waiter got %v after %v", b, time.Since(start))
	}
	close(release)
}

// route: GET /api/v1/explore/music with the engine: its rows join the day's
// plan, no album repeats, and the Browse row has the 8 Browse tiles.
func TestTplExploreAndNewMusic(t *testing.T) {
	r := tplRig(t)
	pages, _ := v7Pages(t, r.owner, "US")
	rows := v7Rows(pages)
	noAlbumTwice(t, rows)
	tpl, grid := 0, 0
	for _, row := range rows {
		if strings.HasPrefix(row.ID, "tpl:") {
			tpl++
			if len(row.Items) < 8 || strings.TrimSpace(row.Reason) == "" {
				t.Fatalf("%s: %d items, reason %q", row.ID, len(row.Items), row.Reason)
			}
		}
		if row.Shape == "grid" {
			grid = len(row.Items)
			for _, c := range row.Items {
				if c.Href == "" {
					t.Fatalf("Browse tile %s has no page", c.Key)
				}
			}
		}
	}
	if tpl < 3 || grid > 8 {
		t.Fatalf("explore: %d engine rows, Browse row %d tiles", tpl, grid)
	}
	var n struct {
		Rows []MusicRow `json:"rows"`
	}
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/explore/new/music?region=US", "", 200), &n); err != nil {
		t.Fatal(err)
	}
	engine := 0
	for _, row := range n.Rows {
		if strings.HasPrefix(row.ID, "tpl:") {
			engine++
		}
	}
	if engine < 3 {
		t.Fatalf("new & popular: %d engine rows", engine)
	}
}

// Every album the engine shows is listed for the picture route (it serves
// only listed ids), Browse tile covers included.
func TestTplCardsAreListedForPictures(t *testing.T) {
	r := tplRig(t)
	var a tplAnswer
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/music/genre/rock?region=US", "", 200), &a); err != nil {
		t.Fatal(err)
	}
	listed := func(kind, id string) bool {
		r.api.music.mu.Lock()
		defer r.api.music.mu.Unlock()
		_, ok := r.api.music.names[kind+"|"+id]
		return ok
	}
	for _, row := range a.Rows {
		for _, c := range row.Items {
			if !listed(c.Kind, c.MBID) {
				t.Fatalf("%s %s (%s) not listed for its picture", c.Kind, c.Name, c.MBID)
			}
		}
	}
	var tiles struct {
		Sections []struct {
			Tiles []struct {
				Covers []string `json:"covers"`
			} `json:"tiles"`
		} `json:"sections"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/music/browse-tiles?region=us", "", 200), &tiles)
	for _, s := range tiles.Sections {
		for _, tl := range s.Tiles {
			for _, c := range tl.Covers {
				if id := strings.TrimPrefix(c, musicArtPrefix+"album/"); !listed("album", id) {
					t.Fatalf("tile cover %s not listed", id)
				}
			}
		}
	}
}
