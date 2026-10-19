package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
