package library

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestArtworkRedirectToOtherHostRefused: a picture host that redirects
// somewhere not allowed gets nothing fetched from there.
func TestArtworkRedirectToOtherHostRefused(t *testing.T) {
	var hit atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer other.Close()
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/p.jpg", http.StatusFound)
	}))
	defer pics.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{pics.URL + "/img/"}, HTTP: &http.Client{}}
	if _, err := c.Fetch(context.Background(), pics.URL+"/img/a.jpg"); err == nil {
		t.Fatal("redirect to a host not allowed was followed")
	}
	if hit.Load() != 0 {
		t.Fatal("the other host was asked")
	}
}

// TestArtworkDefaultClientPublicOnly: without an injected client, pictures
// are never fetched from this machine or the local network.
func TestArtworkDefaultClientPublicOnly(t *testing.T) {
	var hit atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer srv.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{srv.URL + "/"}}
	if _, err := c.Fetch(context.Background(), srv.URL+"/a.jpg"); err == nil || hit.Load() != 0 {
		t.Fatalf("fetched from loopback: %v", err)
	}
	if ArtworkClient().Transport.(*http.Transport).Proxy != nil {
		t.Fatal("artwork client uses a proxy")
	}
}

func TestTMDBArtworkURL(t *testing.T) {
	const base = "https://image.tmdb.org/t/p"
	if u, ok := TMDBArtworkURL(base, "w342", "abc_D-9.jpg"); !ok || u != base+"/w342/abc_D-9.jpg" {
		t.Fatalf("got %q %v", u, ok)
	}
	for _, c := range [][2]string{{"w343", "a.jpg"}, {"w342", "../x.jpg"}, {"w342", "a.gif"}, {"w342", "a.b.jpg"}, {"w342", ".jpg"}, {"w342", "a%2f.jpg"}, {"", "a.jpg"}} {
		if _, ok := TMDBArtworkURL(base, c[0], c[1]); ok {
			t.Errorf("%v accepted", c)
		}
	}
}

// TestArtworkSameFetchOnce: many asks at once for a picture not stored yet
// all get it, and it is downloaded once (two writers to one file broke
// pictures on Windows).
func TestArtworkSameFetchOnce(t *testing.T) {
	var hit atomic.Int32
	release := make(chan struct{})
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Add(1)
		<-release
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer pics.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{pics.URL + "/img/"}, HTTP: &http.Client{}}
	url := pics.URL + "/img/a.jpg"
	const n = 8
	errs := make(chan error, n)
	for range n {
		go func() {
			name, err := c.Fetch(context.Background(), url)
			if err == nil {
				if _, ok := c.Path(name); !ok {
					err = errors.New("stored picture missing")
				}
			}
			errs <- err
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if got := hit.Load(); got != 1 {
		t.Fatalf("downloaded %d times, want once", got)
	}
}
