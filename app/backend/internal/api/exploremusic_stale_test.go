package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// A Library change marks the plan stale; the next visit is answered at once
// from the last good plan while the slow rebuild runs behind, never inline.
func TestExploreMusicV7StaleServedWhileRebuilding(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	first := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	r.api.forgetMusicV7Snaps(-1)
	r.api.musicv2.cand = musicscore.Candidates{} // the rebuild must ask the server
	old := musicV7Wait
	musicV7Wait = 200 * time.Millisecond
	t.Cleanup(func() { musicV7Wait = old })
	slowV7(r, 2*time.Second)
	start := time.Now()
	again := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("stale visit took %v, want the last good plan at once", took)
	}
	if string(v7PageRows(t, first)) != string(v7PageRows(t, again)) {
		t.Fatal("stale visit did not serve the last good plan")
	}
	var a v7Answer
	_ = json.Unmarshal(again, &a)
	if a.Next != "" {
		r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+a.Next, "", 200)
	}
}

// After a restart with a plan on disk that a Library change marked stale,
// the kept plan is still served (not deleted) and rebuilt behind.
func TestExploreMusicV7StaleDiskPlanServed(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	first := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	r.api.forgetMusicV7Snaps(-1)
	restartV7(r)
	// Layer 1 gone too: the rebuild must ask the (slow) metadata server.
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM lookup_cache WHERE key LIKE ?", musicDiskPrefix+"%")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	old := musicV7Wait
	musicV7Wait = 200 * time.Millisecond
	t.Cleanup(func() { musicV7Wait = old })
	n := slowV7(r, 2*time.Second)
	start := time.Now()
	again := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("stale disk plan took %v", took)
	}
	if string(v7PageRows(t, first)) != string(v7PageRows(t, again)) {
		t.Fatal("stale disk plan not served")
	}
	if n.Load() == 0 {
		t.Fatal("stale disk plan not rebuilt behind")
	}
}

// With nothing kept at all, a visitor waits at most about musicV7Wait.
func TestExploreMusicV7ColdDeadline(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	old := musicV7Wait
	musicV7Wait = 300 * time.Millisecond
	t.Cleanup(func() { musicV7Wait = old })
	slowV7(r, 3*time.Second)
	start := time.Now()
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Fatalf("cold visit took %v, deadline %v", took, musicV7Wait)
	}
}

// The answer carries an ETag; asking again with it is a 304.
func TestExploreMusicV7ETag(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	req := httptest.NewRequest("GET", "/api/v1/explore/music?region=US", nil)
	w := httptest.NewRecorder()
	writeJSONTagged(w, req, map[string]any{"rows": []int{1, 2}})
	tag := w.Header().Get("ETag")
	if tag == "" || w.Code != 200 {
		t.Fatalf("no ETag (%d)", w.Code)
	}
	req.Header.Set("If-None-Match", "W/"+tag)
	w = httptest.NewRecorder()
	writeJSONTagged(w, req, map[string]any{"rows": []int{1, 2}})
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("revisit = %d with %d bytes, want 304", w.Code, w.Body.Len())
	}
	ask := func(tag string) *http.Response {
		req, _ := http.NewRequest("GET", r.owner.base+"/api/v1/explore/music?region=US", nil)
		if tag != "" {
			req.Header.Set("If-None-Match", tag)
		}
		resp, err := r.owner.c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	h := ask("")
	if h.Header.Get("ETag") == "" || h.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("explore/music headers: ETag %q, Cache-Control %q", h.Header.Get("ETag"), h.Header.Get("Cache-Control"))
	}
	if got := ask(h.Header.Get("ETag")); got.StatusCode != http.StatusNotModified {
		t.Fatalf("explore/music revisit = %d, want 304", got.StatusCode)
	}
}

// The prewarm builds the member and region pairs asked for lately.
func TestExploreMusicV7PrewarmAsked(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	// The prewarm runs on the wall clock, so the ask is dated by it too (the
	// rig's fixed clock drifts out of the recent window as the days pass).
	now := time.Now().UTC()
	r.api.rememberMusicV7Ask(7, "JP", now)
	r.api.prewarmMusicV7(context.Background())
	k := musicscore.PlanKey{Member: 7, Date: musicscore.Day(now), View: "JP"}
	if _, ok := r.api.musicv2.plans.Current(k, now); !ok {
		t.Fatal("asked member 7 / JP not prewarmed")
	}
}
