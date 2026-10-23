package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// slowV7 makes the rig's metadata server answer after d and counts its
// requests (the fake slow server of the cold-start tests).
func slowV7(r *askRig, d time.Duration) *atomic.Int64 {
	n := &atomic.Int64{}
	l, s, ad := r.api.s.MusicListeners, r.api.s.MusicSimilarSet, r.api.s.MusicArtistData
	r.api.s.MusicListeners = func(c context.Context, ids []string) (map[string]int64, error) {
		n.Add(1)
		time.Sleep(d)
		return l(c, ids)
	}
	r.api.s.MusicSimilarSet = func(c context.Context, ids []string) (map[string][]metadata.MusicSimilarScore, error) {
		n.Add(1)
		time.Sleep(d)
		return s(c, ids)
	}
	r.api.s.MusicArtistData = func(c context.Context, ids []string) ([]metadata.MusicArtistData, error) {
		n.Add(1)
		time.Sleep(d)
		return ad(c, ids)
	}
	return n
}

// restartV7 forgets every in-memory Music v7 layer, as a restart does; the
// store (lookup_cache) stays.
func restartV7(r *askRig) {
	now := r.api.musicv2.now
	r.api.musicv2 = musicV2State{now: now}
	r.api.forgetMusicAnswers()
}

func v7PageRows(t *testing.T, b []byte) []byte {
	t.Helper()
	var a v7Answer
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(a.Rows)
	return out
}

// route: GET /api/v1/explore/music (music_rows_v7 restart from disk)
func TestExploreMusicV7RestartServesPlanFromDisk(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	first := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	restartV7(r)
	n := slowV7(r, 500*time.Millisecond)
	start := time.Now()
	again := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	took := time.Since(start)
	if took > 2*time.Second {
		t.Fatalf("page 1 after restart took %v", took)
	}
	if n.Load() != 0 {
		t.Fatalf("page 1 after restart made %d metadata requests, want 0 (plan on disk)", n.Load())
	}
	if !bytes.Equal(v7PageRows(t, first), v7PageRows(t, again)) {
		t.Fatal("page 1 after restart differs from the kept plan")
	}
	// Its cursor pages on.
	var a v7Answer
	_ = json.Unmarshal(again, &a)
	if a.Next == "" {
		t.Fatal("no next cursor")
	}
	r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+a.Next, "", 200)
	t.Logf("restart page 1 in %v", took)
}

// route: GET /api/v1/explore/music (music_rows_v7 cold start)
func TestExploreMusicV7ColdStartUnderBudget(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	// The prewarm (household build) has run before the restart: layer 1 is
	// on disk, the member has no kept plan of their own.
	r.api.prewarmMusicV7(context.Background())
	restartV7(r)
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM lookup_cache WHERE key LIKE ?", musicSnapPrefix+"%")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	n := slowV7(r, 300*time.Millisecond)
	start := time.Now()
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	took := time.Since(start)
	t.Logf("cold page 1 in %v with %d metadata requests", took, n.Load())
	if took > 2*time.Second {
		t.Fatalf("cold page 1 took %v, limit 2s", took)
	}
	if n.Load() > 25 {
		t.Fatalf("cold page 1 made %d metadata requests, limit 25", n.Load())
	}
}

// route: GET /api/v1/explore/music (music_rows_v7 nothing kept)
func TestExploreMusicV7EmptyStartParallel(t *testing.T) {
	r, _, _ := v7Rig(t, "household_large.json")
	n := slowV7(r, 100*time.Millisecond)
	start := time.Now()
	r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	took := time.Since(start)
	t.Logf("empty page 1 in %v with %d metadata requests", took, n.Load())
	// 35 requests of 100 ms one after another were 3.5 s; chunks now go 4 at a time.
	if took > 2*time.Second {
		t.Fatalf("empty page 1 took %v, limit 2s", took)
	}
}
