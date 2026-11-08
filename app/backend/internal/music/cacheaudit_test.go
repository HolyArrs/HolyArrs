package music

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Cache audit 23 October 2026, finding 7: a genre tile without a content version
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
	for i := 0; i <= GenreArtMax; i++ {
		req := httptest.NewRequest("GET", "/api/v1/music/genre-art/pop?v="+time.Duration(i).String(), nil)
		req.SetPathValue("slug", "pop")
		r.api.genreArtHandlers().Art(httptest.NewRecorder(), req)
	}
	if n := r.api.genreArt.Len(); n < GenreArtMax*8/10 || n > GenreArtMax {
		t.Fatalf("size %d", n)
	}
}

// Finding 10: an empty layer-1 value (no similar artists, 0 listeners, no
// genres or albums) is not kept a week, on disk or in memory.
func TestMusicBatchEmptyNotKeptLong(t *testing.T) {
	t.Parallel()
	a := storeAPI(t)
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

// storeAPI is a music Handlers over an empty store.
func storeAPI(t *testing.T) *API {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(Server{Store: st})
}
