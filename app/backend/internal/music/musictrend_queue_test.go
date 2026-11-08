package music

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music/rowtpl"
)

// The picture route answers from disk only: a picture not stored here yet
// is a 204 at once while it is fetched behind the answer, and the next ask
// serves it.
func TestPictureRouteNeverWaitsOnFetch(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const id = "55555555-5555-4555-8555-555555555555"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(700 * time.Millisecond) // a slow metadata server
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff stored"))
	}))
	defer srv.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: srv.Client()}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artwork: art}, ImageBase: srv.URL + "/v1/img", Version: "test"})
	b, _ := rowtpl.ParseBundle([]byte(`{"version":"t","items":[{"id":"` + id + `","k":"a","t":"Slow","ar":"S"}],"keys":{},"names":{}}`))
	a.musicPre.catalog.Store(b)
	ask := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/music-trending/art/album/"+id+"?size=500", nil)
		r.SetPathValue("kind", "album")
		r.SetPathValue("mbid", id)
		w := httptest.NewRecorder()
		a.musicTrendArt(w, r)
		return w
	}
	start := time.Now()
	w := ask()
	if d := time.Since(start); w.Code != http.StatusNoContent || d > 300*time.Millisecond {
		t.Fatalf("first ask %d in %v, want a 204 at once", w.Code, d)
	}
	if w.Header().Get("Cache-Control") != "private, max-age=20" {
		t.Fatalf("queued miss cache %q", w.Header().Get("Cache-Control"))
	}
	ask() // asked again while queued: no second fetch
	a.music.prefetching.Wait()
	if w := ask(); w.Code != http.StatusOK {
		t.Fatalf("after the fetch %d, want the stored picture", w.Code)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("metadata server asked %d times", n)
	}
}

// A picture missed again and again is asked less and less often.
func TestPictureMissRetrySchedule(t *testing.T) {
	t.Parallel()
	want := []time.Duration{10 * time.Minute, 10 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 6 * time.Hour}
	for n, d := range want {
		if got := artMissWait(n); got != d {
			t.Fatalf("artMissWait(%d) = %v, want %v", n, got, d)
		}
	}
}
