package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// Cache audit 2026-10-23, finding 7: a genre tile without a content version
// is not immutable for a week.
func TestGenreArtImmutableOnlyWithVersion(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	r.api.s.MusicGenreTile = func(_ context.Context, slug, v string) ([]byte, error) { return []byte("\xff\xd8jpeg"), nil }
	for v, want := range map[string]string{"ab12": "immutable", "": "max-age=3600"} {
		req := httptest.NewRequest("GET", "/api/v1/music/genre-art/pop?v="+v, nil)
		req.SetPathValue("slug", "pop")
		w := httptest.NewRecorder()
		r.api.genreArtHandlers().Art(w, req)
		cc := w.Header().Get("Cache-Control")
		if w.Code != 200 || !strings.Contains(cc, want) || (v == "" && strings.Contains(cc, "immutable")) {
			t.Fatalf("v=%q: %d %q", v, w.Code, cc)
		}
	}
}

// Finding 15: the genre picture cache drops its oldest entries at the cap.
func TestGenreArtCacheEvictsOldest(t *testing.T) {
	t.Parallel()
	r := newMusicAskRig(t)
	r.api.s.MusicGenreTile = func(_ context.Context, slug, v string) ([]byte, error) { return []byte("\xff\xd8jpeg"), nil }
	for i := 0; i <= music.GenreArtMax; i++ {
		req := httptest.NewRequest("GET", "/api/v1/music/genre-art/pop?v="+time.Duration(i).String(), nil)
		req.SetPathValue("slug", "pop")
		r.api.genreArtHandlers().Art(httptest.NewRecorder(), req)
	}
	if n := r.api.genreArt.Len(); n < music.GenreArtMax*8/10 || n > music.GenreArtMax {
		t.Fatalf("size %d", n)
	}
}

// Finding 5: a picture from a stable address is not immutable for a year.
func TestServeArtworkStableRevalidates(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// Findings 12 and 14: card art 7 days; person and collection lists a day.
func TestShorterListAndCardKeeps(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	if d := detailsTTL(discoverEntry{Details: &metadata.Details{}}); d > 15*time.Minute {
		t.Fatalf("no videos kept %v", d)
	}
	if d := detailsTTL(discoverEntry{Details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "k"}}}}); d != discoverTTL["details"] {
		t.Fatalf("videos kept %v", d)
	}
}
