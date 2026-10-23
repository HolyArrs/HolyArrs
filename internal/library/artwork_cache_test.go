package library

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Cache audit 2026-10-23, finding 4: the metadata server's grey stand-in is
// never stored, whichever caller fetches it.
func TestFetchRefusesServerPlaceholder(t *testing.T) {
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Metadata-Placeholder", "true")
		_, _ = w.Write([]byte("grey"))
	}))
	defer pics.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	if _, err := c.Fetch(context.Background(), pics.URL+"/v1/img/artist/x/500.jpg"); !errors.Is(err, ErrArtworkRejected) {
		t.Fatalf("placeholder stored: %v", err)
	}
}

// Finding 5: a cover at an address that does not change with the picture
// (Cover Art Archive, the server's picture routes) is re-checked daily in
// the background with its ETag (304 = no download); TMDB names are content
// hashes and are never re-asked.
func TestStableAddressCoverRecheckedDaily(t *testing.T) {
	var asked, full atomic.Int32
	body := atomic.Value{}
	body.Store("one")
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		b := body.Load().(string)
		w.Header().Set("ETag", `"`+b+`"`)
		if r.Header.Get("If-None-Match") == `"`+b+`"` {
			w.WriteHeader(304)
			return
		}
		full.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte(b))
	}))
	defer pics.Close()
	dir := t.TempDir()
	c := &ArtworkCache{Dir: dir, Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	cover, tmdb := pics.URL+"/release-group/x/front-250", pics.URL+"/t/p/w342/abc.jpg"
	name, _ := c.Fetch(context.Background(), cover)
	_, _ = c.Fetch(context.Background(), tmdb)
	if !StableArtworkURL(cover) || StableArtworkURL(tmdb) || StableArtworkURL(pics.URL+"/v1/img/artist/x/500.jpg?v=abc") || !StableArtworkURL(pics.URL+"/v1/img/artist/x/500.jpg") {
		t.Fatal("stable address rule")
	}
	// Fresh: nothing is asked.
	asked.Store(0)
	c.Recheck(cover, nil)
	c.Recheck(tmdb, nil)
	time.Sleep(200 * time.Millisecond)
	if asked.Load() != 0 {
		t.Fatalf("fresh re-asked %d", asked.Load())
	}
	age := func() {
		old := time.Now().Add(-25 * time.Hour)
		for _, e := range []string{name, ArtworkName(tmdb)} {
			_ = os.Chtimes(filepath.Join(dir, checkName(e)), old, old)
		}
	}
	wait := func(want int32) {
		deadline := time.Now().Add(5 * time.Second)
		for asked.Load() < want && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// A day old, unchanged: one cheap 304, the TMDB picture is not asked.
	age()
	full.Store(0)
	c.Recheck(cover, nil)
	c.Recheck(tmdb, nil)
	wait(1)
	if asked.Load() != 1 || full.Load() != 0 {
		t.Fatalf("unchanged: asked %d, full %d", asked.Load(), full.Load())
	}
	// Changed upstream: the new picture replaces the old one.
	body.Store("two")
	age()
	asked.Store(0)
	c.Recheck(cover, nil)
	wait(1)
	if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != "two" {
		t.Fatalf("not replaced: %q", b)
	}
}
