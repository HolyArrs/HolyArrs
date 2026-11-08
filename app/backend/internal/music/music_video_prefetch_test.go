package music

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/trailer/batch
// An artist named in the in-view batch is only noted: nothing is
// pending and no lookup starts; the trailer pass makes its music video
// ready, the batch then has it, and the artist page answers from it
// without a lookup. The pass keeps it ready: a nearly expired stream is
// renewed, and a renewal that fails keeps the address it has.
func TestArtistVideoPrefetch(t *testing.T) {
	yt := fakeReader(t, "heat", "dQw4w9WgXcQ")
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ", MostReplayed: 42}}, nil
	})
	base := u[:strings.Index(u, "/api/v1/")]
	id := "artist:" + apiArtist
	var b trailerBatchView
	if err := json.Unmarshal(do(t, "GET", base+"/api/v1/trailer/batch?ids="+id, "", 200), &b); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if len(b.Pending) != 0 || len(b.Videos) != 0 || yt.runs.Load() != 0 {
		t.Fatalf("first batch %+v, %d reader calls", b, yt.runs.Load())
	}
	if n := videoPass(t, u); n != 1 {
		t.Fatalf("pass %d", n)
	}
	b = trailerBatchView{}
	if err := json.Unmarshal(do(t, "GET", base+"/api/v1/trailer/batch?ids="+id, "", 200), &b); err != nil {
		t.Fatal(err)
	}
	v := b.Videos[id]
	if v.Key != "dQw4w9WgXcQ" || v.Start != 42 || !strings.HasSuffix(v.ProxyURL, "/manifest.m3u8") || len(b.Pending) != 0 {
		t.Fatalf("batch %+v", b)
	}
	before := yt.runs.Load()
	var got ArtistVideoView
	_ = json.Unmarshal(do(t, "GET", u, "", 200), &got)
	if got.Key != "dQw4w9WgXcQ" || got.ProxyURL == "" || yt.runs.Load() != before {
		t.Fatalf("page video %+v, lookups %d -> %d", got, before, yt.runs.Load())
	}
	// Ready and not near expiry: the next pass leaves it.
	if n := videoPass(t, u); n != 0 {
		t.Fatalf("looked up again: %d", n)
	}
	// Near expiry: renewed. A renewal YouTube refuses keeps the address.
	a := apiOf(t, u)
	now := time.Now()
	soon := now.Add(metadata.TrailerEarly + time.Minute)
	a.Trailers.KeepStream(context.Background(), "musicvideo|dQw4w9WgXcQ", metadata.TrailerStream{Type: "hls", URL: "https://manifest.googlevideo.com/api/manifest/hls_playlist/id/x", ExpiresAt: soon, Key: "dQw4w9WgXcQ"}, now)
	yt.set("dQw4w9WgXcQ", "bot")
	if n := videoPass(t, u); n != 1 {
		t.Fatalf("renewal pass %d", n)
	}
	if s, _ := a.videoKnown(context.Background(), "dQw4w9WgXcQ"); s.URL == "" {
		t.Fatal("a refused renewal dropped the address")
	}
	yt.set("dQw4w9WgXcQ", "heat")
	a.Trailers.ResetDoor()
	if n := videoPass(t, u); n != 1 {
		t.Fatalf("second renewal pass %d", n)
	}
	if s, _ := a.videoKnown(context.Background(), "dQw4w9WgXcQ"); !s.ExpiresAt.After(soon) {
		t.Fatalf("not renewed: %+v", s)
	}
	// Not a MusicBrainz id.
	do(t, "GET", base+"/api/v1/trailer/batch?ids=artist:nope", "", 400)
}

// apiOf is the API behind a videoSetup address.
func apiOf(t *testing.T, u string) *API {
	t.Helper()
	v, ok := videoAPIs.Load(u[:strings.Index(u, "/api/v1/")])
	if !ok {
		t.Fatal("no API")
	}
	return v.(*API)
}

// The artists kept ready survive a restart (a new API on the same store),
// keep their order, and when full the one asked for least lately goes.
func TestArtistAskedKeptAndBounded(t *testing.T) {
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) { return nil, nil })
	a := apiOf(t, u)
	ctx := context.Background()
	ids := make([]string, 0, artistAskedMax)
	for i := range artistAskedMax {
		ids = append(ids, fmt.Sprintf("a0000000-0000-0000-0000-%012d", i))
	}
	a.noteArtistVideos(ctx, ids)
	time.Sleep(2 * time.Millisecond)
	a.noteArtistVideos(ctx, []string{ids[0]})
	time.Sleep(2 * time.Millisecond)
	a.noteArtistVideos(ctx, []string{"b0000000-0000-0000-0000-000000000001", ids[2]})
	got := a.artistAsks(ctx, time.Now())
	if len(got) != artistAskedMax || got[0] != ids[0] || got[1] != ids[2] || got[len(got)-1] != "b0000000-0000-0000-0000-000000000001" {
		t.Fatalf("%d kept, first %s %s, last %s", len(got), got[0], got[1], got[len(got)-1])
	}
	a.saveArtistAsks(ctx)
	b := New(Server{Store: a.s.Store, Library: a.s.Library, ArtistVideos: a.s.ArtistVideos, Metadata: &metadata.Static{}})
	if again := b.artistAsks(ctx, time.Now()); !slices.Equal(again, got) {
		t.Fatalf("after a restart %d kept, first %v", len(again), again[:2])
	}
	if old := b.artistAsks(ctx, time.Now().Add(artistAskedKeep+time.Minute)); len(old) != 0 {
		t.Fatalf("old ones kept: %d", len(old))
	}
}

// A noted artist with no videos is asked of the metadata server again about
// once a day by the pass, not on every pass.
func TestArtistNoVideosAskedDaily(t *testing.T) {
	asks := 0
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) { asks++; return nil, nil })
	a := apiOf(t, u)
	ctx := context.Background()
	clock := time.Now()
	a.artistVideos.mu.Lock()
	a.artistVideos.now = func() time.Time { return clock }
	a.artistVideos.mu.Unlock()
	a.noteArtistVideos(ctx, []string{apiArtist})
	// Passes 6 minutes apart (past the page's 5-minute wait for an empty list).
	for range 5 {
		videoPass(t, u)
		clock = clock.Add(6 * time.Minute)
	}
	if asks != 1 {
		t.Fatalf("asked %d times in 5 passes", asks)
	}
	// A day later it is asked again.
	clock = clock.Add(artistEmptyRecheck)
	videoPass(t, u)
	if asks != 2 {
		t.Fatalf("not asked again after a day: %d", asks)
	}
}

// The metadata server down: a noted artist is asked again after about an
// hour, not on every pass; and an artist with no videos stays known across
// a restart (a new API on the same store does not ask at once).
func TestArtistOutageAndEmptyKept(t *testing.T) {
	asks := 0
	down := true
	u := videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		asks++
		if down {
			return nil, metadata.ErrServiceUnavailable
		}
		return nil, nil
	})
	a := apiOf(t, u)
	ctx := context.Background()
	clock := time.Now()
	a.artistVideos.mu.Lock()
	a.artistVideos.now = func() time.Time { return clock }
	a.artistVideos.mu.Unlock()
	a.noteArtistVideos(ctx, []string{apiArtist})
	for range 5 {
		videoPass(t, u)
		clock = clock.Add(6 * time.Minute)
	}
	if asks != 1 {
		t.Fatalf("server down: asked %d times in 5 passes", asks)
	}
	clock = clock.Add(artistFailWait)
	down = false
	videoPass(t, u)
	if asks != 2 {
		t.Fatalf("not asked again after an hour: %d", asks)
	}
	// Now kept as "no videos": a restart (new API, same store) does not ask.
	b := New(Server{Store: a.s.Store, Library: a.s.Library, ArtistVideos: a.s.ArtistVideos, Metadata: &metadata.Static{}})
	if due, _, _ := b.artistVideoDue(ctx, apiArtist, time.Now()); due || asks != 2 {
		t.Fatalf("after a restart: due %v, asked %d", due, asks)
	}
}

// The failures kept for the pass are bounded: old ones go, the map never
// passes artistVideosMax, and an answer clears an artist's failure.
func TestArtistFailedBounded(t *testing.T) {
	var c artistVideoCache
	now := time.Now()
	for i := range artistVideosMax + 50 {
		c.noteFailed(fmt.Sprintf("a0000000-0000-0000-0000-%012d", i), now)
	}
	if n := len(c.failed); n != artistVideosMax {
		t.Fatalf("%d failures kept", n)
	}
	later := now.Add(artistFailWait + time.Minute)
	c.noteFailed("b0000000-0000-0000-0000-000000000001", later)
	if n := len(c.failed); n != 1 {
		t.Fatalf("old failures not pruned: %d", n)
	}
	c.forgetFailed("b0000000-0000-0000-0000-000000000001")
	if n := len(c.failed); n != 0 {
		t.Fatalf("an answer did not clear the failure: %d", n)
	}
}
