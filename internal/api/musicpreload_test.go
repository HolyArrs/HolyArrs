package api

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

// Luis's rule: every cover a music page shows is stored before anyone opens
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
