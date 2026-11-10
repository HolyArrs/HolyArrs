package music

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitPreload(t *testing.T, a *API) musicPreloadStatus {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if st := a.tplPreloadStatus(); !st.Running && st.Total > 0 {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("preload did not finish")
	return musicPreloadStatus{}
}

// The rule: every cover a music page shows is stored before anyone opens
// it. The preload warms every shown Browse page, the tile covers, Explore's
// engine rows and New & Popular; a member's first visit finds every card
// warmed, and asking again for the same region and bundle warms nothing new.
func TestTplPreloadWarmsEveryShownCover(t *testing.T) {
	r := tplRig(t)
	on, gap := musicPreloadOn, musicPreloadGap
	musicPreloadOn, musicPreloadGap = true, 0
	defer func() { musicPreloadOn, musicPreloadGap = on, gap }()
	var mu sync.Mutex
	warmed := map[string]bool{}
	r.api.musicPre.warm = func(_ context.Context, k preloadKey) bool {
		mu.Lock()
		defer mu.Unlock()
		warmed[k.kind+"|"+k.mbid] = true
		return true
	}
	b := r.api.musicBundle(context.Background(), time.Now().UTC())
	r.api.tplPreload(b, "US")
	st := waitPreload(t, r.api)
	if st.Warmed != st.Total || st.Failed != 0 || st.Version != b.Version {
		t.Fatalf("status %+v", st)
	}
	mu.Lock()
	n := len(warmed)
	mu.Unlock()
	if st.Total != n {
		t.Fatalf("status total %d, warmed %d", st.Total, n)
	}
	isWarm := func(kind, id string) bool {
		mu.Lock()
		defer mu.Unlock()
		return warmed[kind+"|"+id]
	}
	check := func(where string, cards []MusicCard) {
		t.Helper()
		for _, c := range cards {
			if !isWarm(c.Kind, c.MBID) {
				t.Fatalf("%s: %s %s (%s) shown cold", where, c.Kind, c.Name, c.MBID)
			}
		}
	}
	var tiles struct {
		Sections []struct {
			Tiles []struct {
				Key    string   `json:"key"`
				Covers []string `json:"covers"`
			} `json:"tiles"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/music/browse-tiles?region=US", "", 200), &tiles); err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, s := range tiles.Sections {
		for _, tl := range s.Tiles {
			for _, c := range tl.Covers {
				if id := strings.TrimPrefix(c, musicArtPrefix+"album/"); !isWarm("album", id) {
					t.Fatalf("tile %s cover %s cold", tl.Key, id)
				}
			}
			kind, slug, _ := strings.Cut(tl.Key, ":")
			if kind == "decade" {
				slug = slug[2:]
			}
			if tl.Key == "activity:party" {
				kind, slug = "mood", "party"
			}
			path := "/api/v1/music/browse/" + kind + "/" + slug + "?region=US"
			if kind == "genre" {
				path = "/api/v1/music/genre/" + slug + "?region=US"
			}
			var p tplAnswer
			if err := json.Unmarshal(r.owner.call("GET", path, "", 200), &p); err != nil {
				t.Fatal(err)
			}
			if p.Hero != nil {
				check(path+" spotlight", append([]MusicCard{*p.Hero}, p.Alts...))
			}
			for _, row := range p.Rows {
				check(path+" "+row.ID, row.Items)
			}
			pages++
		}
	}
	var np struct {
		Hero []MusicCard `json:"hero"`
		Rows []MusicRow  `json:"rows"`
	}
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/explore/new/music?region=US", "", 200), &np); err != nil {
		t.Fatal(err)
	}
	for _, row := range np.Rows {
		if strings.HasPrefix(row.ID, "tpl:") {
			check("new "+row.ID, row.Items)
		}
	}
	if pages == 0 {
		t.Fatal("no Browse page shown")
	}
	r.api.tplPreload(b, "US")
	time.Sleep(50 * time.Millisecond)
	if again := r.api.tplPreloadStatus(); again.Total != st.Total || again.Running {
		t.Fatalf("the same region and bundle preloaded again: %+v", again)
	}
}

// A picture the server has not stored yet is a miss, asked again later in
// the background (never from an outside source).
func TestTplPreloadAsksMissesAgain(t *testing.T) {
	r := tplRig(t)
	on, gap, retry := musicPreloadOn, musicPreloadGap, musicPreloadRetry
	musicPreloadOn, musicPreloadGap, musicPreloadRetry = true, 0, 50*time.Millisecond
	defer func() { musicPreloadOn, musicPreloadGap, musicPreloadRetry = on, gap, retry }()
	var mu sync.Mutex
	tries := map[preloadKey]int{}
	r.api.musicPre.warm = func(_ context.Context, k preloadKey) bool {
		mu.Lock()
		defer mu.Unlock()
		tries[k]++
		return tries[k] > 1 // stored by the second ask
	}
	r.api.tplPreload(r.api.musicBundle(context.Background(), time.Now().UTC()), "US")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if st := r.api.tplPreloadStatus(); !st.Running && st.Total > 0 && st.Warmed == st.Total && st.Failed == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("misses not asked again: %+v", r.api.tplPreloadStatus())
}

// Once the preload has stored pictures, every engine row puts its items
// with a stored picture first, so initials show only at a row's tail and
// the page keeps its rows.
func TestTplRowsPutStoredPicturesFirst(t *testing.T) {
	r := tplRig(t)
	on, gap, retry := musicPreloadOn, musicPreloadGap, musicPreloadRetry
	musicPreloadOn, musicPreloadGap, musicPreloadRetry = true, 0, time.Hour
	defer func() { musicPreloadOn, musicPreloadGap, musicPreloadRetry = on, gap, retry }()
	var full tplAnswer
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/music/genre/rock?region=US", "", 200), &full); err != nil {
		t.Fatal(err)
	}
	missing := func(k preloadKey) bool { return k.kind == "album" && k.mbid[len(k.mbid)-1]%3 == 0 }
	r.api.musicPre.warm = func(_ context.Context, k preloadKey) bool { return !missing(k) }
	r.api.tplPreload(r.api.musicBundle(context.Background(), time.Now().UTC()), "US")
	waitPreload(t, r.api)
	var p tplAnswer
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/music/genre/rock?region=US", "", 200), &p); err != nil {
		t.Fatal(err)
	}
	// The day's row order decides how many rows fold together, so the count moves a little from day to day.
	if len(p.Rows)*3 < len(full.Rows)*2 {
		t.Fatalf("%d rows, %d before: the page must keep most of its rows", len(p.Rows), len(full.Rows))
	}
	for _, row := range p.Rows {
		bare := false
		for _, c := range row.Items {
			if missing(preloadKey{c.Kind, c.MBID}) {
				bare = true
			} else if bare {
				t.Fatalf("%s: %s with a picture after one without", row.ID, c.Name)
			}
		}
	}
}
