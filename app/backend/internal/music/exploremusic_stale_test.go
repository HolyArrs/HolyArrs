package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// A Library change marks the plan stale; the next visit is answered at once
// from the last good plan while the slow rebuild runs behind, never inline.
func TestExploreMusicStaleServedWhileRebuilding(t *testing.T) {
	r, _, _ := planRig(t, "household_large.json")
	first := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	r.api.forgetMusicSnaps(-1)
	r.api.exploreCache.cand = musicscore.Candidates{} // the rebuild must ask the server
	old := musicWait
	musicWait = 200 * time.Millisecond
	t.Cleanup(func() { musicWait = old })
	slowPlan(r, 2*time.Second)
	start := time.Now()
	again := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("stale visit took %v, want the last good plan at once", took)
	}
	if string(planPageRows(t, first)) != string(planPageRows(t, again)) {
		t.Fatal("stale visit did not serve the last good plan")
	}
	var a planAnswer
	_ = json.Unmarshal(again, &a)
	if a.Next != "" {
		r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+a.Next, "", 200)
	}
}

// After a restart with a plan on disk that a Library change marked stale,
// the kept plan is still served (not deleted) and rebuilt behind.
func TestExploreMusicStaleDiskPlanServed(t *testing.T) {
	r, _, _ := planRig(t, "household_large.json")
	first := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	r.api.forgetMusicSnaps(-1)
	restartPlan(r)
	// Layer 1 gone too: the rebuild must ask the (slow) metadata server.
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM lookup_cache WHERE key LIKE ?", musicDiskPrefix+"%")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	old := musicWait
	musicWait = 200 * time.Millisecond
	t.Cleanup(func() { musicWait = old })
	n := slowPlan(r, 2*time.Second)
	start := time.Now()
	again := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("stale disk plan took %v", took)
	}
	if string(planPageRows(t, first)) != string(planPageRows(t, again)) {
		t.Fatal("stale disk plan not served")
	}
	if n.Load() == 0 {
		t.Fatal("stale disk plan not rebuilt behind")
	}
}

// With nothing kept at all, a visitor waits at most about musicWait.
func TestExploreMusicColdDeadline(t *testing.T) {
	r, _, _ := planRig(t, "household_large.json")
	old := musicWait
	musicWait = 300 * time.Millisecond
	t.Cleanup(func() { musicWait = old })
	slowPlan(r, 3*time.Second)
	start := time.Now()
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Fatalf("cold visit took %v, deadline %v", took, musicWait)
	}
}

// The answer carries an ETag; asking again with it is a 304.
func TestExploreMusicETag(t *testing.T) {
	r, _, _ := planRig(t, "household_large.json")
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
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
func TestExploreMusicPrewarmAsked(t *testing.T) {
	r, _, _ := planRig(t, "household_large.json")
	// The prewarm runs on the wall clock, so the ask is dated by it too (the
	// rig's fixed clock drifts out of the recent window as the days pass).
	now := time.Now().UTC()
	r.api.rememberMusicAsk(7, "JP", now)
	r.api.prewarmMusicPlan(context.Background())
	k := musicscore.PlanKey{Member: 7, Date: musicscore.Day(now), View: "JP"}
	if _, ok := r.api.exploreCache.plans.Current(k, now); !ok {
		t.Fatal("asked member 7 / JP not prewarmed")
	}
}
