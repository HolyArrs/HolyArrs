package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Cache audit 2026-10-23, finding 7: a genre tile without a content version
// is not immutable for a week.
func TestGenreArtImmutableOnlyWithVersion(t *testing.T) {
	r := newMusicAskRig(t)
	r.api.s.MusicGenreTile = func(_ context.Context, slug, v string) ([]byte, error) { return []byte("\xff\xd8jpeg"), nil }
	for v, want := range map[string]string{"ab12": "immutable", "": "max-age=3600"} {
		req := httptest.NewRequest("GET", "/api/v1/music/genre-art/pop?v="+v, nil)
		req.SetPathValue("slug", "pop")
		w := httptest.NewRecorder()
		r.api.musicGenreArt(w, req)
		cc := w.Header().Get("Cache-Control")
		if w.Code != 200 || !strings.Contains(cc, want) || (v == "" && strings.Contains(cc, "immutable")) {
			t.Fatalf("v=%q: %d %q", v, w.Code, cc)
		}
	}
}

// Finding 15: the genre picture cache drops its oldest entries at the cap.
func TestGenreArtCacheEvictsOldest(t *testing.T) {
	r := newMusicAskRig(t)
	r.api.s.MusicGenreTile = func(_ context.Context, slug, v string) ([]byte, error) { return []byte("\xff\xd8jpeg"), nil }
	for i := 0; i <= genreArtMax; i++ {
		req := httptest.NewRequest("GET", "/api/v1/music/genre-art/pop?v="+time.Duration(i).String(), nil)
		req.SetPathValue("slug", "pop")
		r.api.musicGenreArt(httptest.NewRecorder(), req)
	}
	if n := len(r.api.genreArt.pics); n < genreArtMax*8/10 || n > genreArtMax {
		t.Fatalf("size %d", n)
	}
}

// Finding 15: the extras cache drops its oldest entries at the cap.
func TestExtrasCacheEvictsOldest(t *testing.T) {
	m := map[string]extrasEntry{}
	base := time.Now()
	for i := 0; i < 5001; i++ {
		keepUnder(m, 5000, func(e extrasEntry) time.Time { return e.at })
		m[strconv.Itoa(i)] = extrasEntry{at: base.Add(time.Duration(i) * time.Second)}
	}
	if len(m) < 4000 || len(m) > 5000 {
		t.Fatalf("size %d", len(m))
	}
	if _, ok := m["5"]; ok {
		t.Fatal("oldest kept")
	}
	if _, ok := m["5000"]; !ok {
		t.Fatal("newest dropped")
	}
}

// Finding 5: a picture from a stable address is not immutable for a year.
func TestServeArtworkStableRevalidates(t *testing.T) {
	dir := t.TempDir()
	name := "0123456789abcdef0123456789abcdef.jpg"
	p := filepath.Join(dir, name)
	_ = os.WriteFile(p, []byte("jpeg"), 0o644)
	w := httptest.NewRecorder()
	serveArtwork(w, httptest.NewRequest("GET", "/x", nil), name, p)
	if !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("hashed address %q", w.Header().Get("Cache-Control"))
	}
	_ = os.WriteFile(filepath.Join(dir, ".chk-"+name), []byte(`"e1"`), 0o644)
	w = httptest.NewRecorder()
	serveArtwork(w, httptest.NewRequest("GET", "/x", nil), name, p)
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=86400" || w.Header().Get("ETag") == `"0123456789abcdef0123456789abcdef"` {
		t.Fatalf("stable address %q %q", cc, w.Header().Get("ETag"))
	}
}

// Finding 10: an empty layer-1 value (no similar artists, 0 listeners, no
// genres or albums) is not kept a week, on disk or in memory.
func TestMusicBatchEmptyNotKeptLong(t *testing.T) {
	a := videoAPI(t, nil, nil)
	calls := 0
	fetch := func(_ context.Context, ids []string) (map[string]any, error) {
		calls++
		return map[string]any{"a": []metadata.MusicSimilarScore{}, "b": []metadata.MusicSimilarScore{{}}}, nil
	}
	now := time.Now()
	a.musicBatch(context.Background(), "lb:s:", []string{"a", "b"}, 12, now, fetch)
	if got := a.musicDiskRead(context.Background(), "lb:s:", []string{"a", "b"}, now); len(got) != 1 || got["b"] == nil {
		t.Fatalf("disk %v", got)
	}
	a.musicBatch(context.Background(), "lb:s:", []string{"a", "b"}, 12, now.Add(11*time.Minute), fetch)
	if calls != 2 {
		t.Fatalf("empty kept: calls %d", calls)
	}
}

// Finding 11: a found album cover survives a restart (disk) and is kept 7
// days, not 30.
func TestAlbumCoverOnDiskSevenDays(t *testing.T) {
	st, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a := New(Server{Store: st, MusicCover: func(context.Context, string, string, string) (string, error) { return "https://c/1.jpg", nil }})
	it := metadata.MusicTrendItem{Title: "T", Artist: "A"}
	if src, ok := a.albumCover(context.Background(), apiAlbum, it); !ok || src != "https://c/1.jpg" {
		t.Fatalf("first %q %v", src, ok)
	}
	calls := 0
	b := New(Server{Store: st, MusicCover: func(context.Context, string, string, string) (string, error) {
		calls++
		return "https://c/2.jpg", nil
	}})
	if src, ok := b.albumCover(context.Background(), apiAlbum, it); !ok || src != "https://c/1.jpg" || calls != 0 {
		t.Fatalf("after restart %q %v calls %d", src, ok, calls)
	}
	if musicCoverKeep != 7*24*time.Hour {
		t.Fatalf("keep %v", musicCoverKeep)
	}
}

// Findings 12 and 14: card art 7 days; person and collection lists a day.
func TestShorterListAndCardKeeps(t *testing.T) {
	if cardArtTTL != 7*24*time.Hour {
		t.Fatalf("card art %v", cardArtTTL)
	}
	if exploreListTTL["person"] != 24*time.Hour || exploreListTTL["collection"] != 24*time.Hour {
		t.Fatalf("lists %v", exploreListTTL)
	}
}

// Details with no trailer are asked again within minutes (the server may
// add a season trailer); full details keep the day.
func TestDetailsWithoutVideosShort(t *testing.T) {
	if d := detailsTTL(discoverEntry{details: &metadata.Details{}}); d > 15*time.Minute {
		t.Fatalf("no videos kept %v", d)
	}
	if d := detailsTTL(discoverEntry{details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "k"}}}}); d != discoverTTL["details"] {
		t.Fatalf("videos kept %v", d)
	}
}
