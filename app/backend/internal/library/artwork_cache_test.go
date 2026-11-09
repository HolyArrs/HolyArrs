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

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Cache audit 23 October 2026, finding 4: the metadata server's grey stand-in is
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

// FetchStored takes only a picture the address itself answers: the
// metadata server redirects an album it has not stored, and that answer is
// not kept (the caller's other sources run instead).
func TestFetchStoredRefusesARedirect(t *testing.T) {
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/img/album/moved/500.jpg" {
			http.Redirect(w, r, "/elsewhere.jpg", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("cover"))
	}))
	defer pics.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	ctx := context.Background()
	if name, err := c.FetchStored(ctx, pics.URL+"/v1/img/album/kept/500.jpg", nil); err != nil {
		t.Fatal(err)
	} else if _, ok := c.Path(name); !ok {
		t.Fatal("stored picture not kept")
	}
	moved := pics.URL + "/v1/img/album/moved/500.jpg"
	if _, err := c.FetchStored(ctx, moved, nil); err == nil {
		t.Fatal("redirect taken")
	}
	if _, ok := c.Path(ArtworkName(moved)); ok {
		t.Fatal("redirected picture kept")
	}
}

// Listing a big cache takes seconds: the cache is listed once to count it,
// then only when a download takes the count over the cap, and that trim
// goes down to 90% of the cap so the next downloads list nothing.
func TestTrimListsOnlyWhenOverTheCap(t *testing.T) {
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(make([]byte, 1000))
	}))
	defer pics.Close()
	dir := t.TempDir()
	c := &ArtworkCache{Dir: dir, Cap: 5500, Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	ctx := context.Background()
	fetch := func(p string) {
		t.Helper()
		if _, err := c.Fetch(ctx, pics.URL+"/p/"+p+".jpg"); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		fetch(p)
	}
	if n := c.trimCount(); n != 1 {
		t.Fatalf("%d listings under the cap, want 1 (the first count)", n)
	}
	fetch("f") // 6000 > 5500: one trim, down to 4950 or less
	if n := c.trimCount(); n != 2 {
		t.Fatalf("%d listings, want 2", n)
	}
	es, _ := os.ReadDir(dir)
	total := 0
	for _, e := range es {
		if in, err := e.Info(); err == nil && !e.IsDir() && e.Name()[0] != '.' {
			total += int(in.Size())
		}
	}
	if total > 4950 {
		t.Fatalf("cache holds %d bytes after the trim, want 90%% of the cap or less", total)
	}
	fetch("g") // back to 5000: no listing
	if n := c.trimCount(); n != 2 {
		t.Fatalf("%d listings after a download under the cap, want 2", n)
	}
}

// A background download (preload or warm: the context is marked ahead)
// tells the metadata server nobody waits (X-Fetch: ahead), so it never
// becomes urgent work there; a page's own ask carries no mark.
func TestBackgroundDownloadIsMarked(t *testing.T) {
	var marks []string
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marks = append(marks, r.Header.Get("X-Fetch"))
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("cover"))
	}))
	defer pics.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}
	if _, err := c.FetchStored(metadata.Ahead(context.Background()), pics.URL+"/v1/img/album/a/250.jpg", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FetchStored(context.Background(), pics.URL+"/v1/img/album/b/250.jpg", nil); err != nil {
		t.Fatal(err)
	}
	if len(marks) != 2 || marks[0] != "ahead" || marks[1] != "" {
		t.Fatalf("marks %q", marks)
	}
}
