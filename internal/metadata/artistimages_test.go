package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fakeDeezer(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/search/artist" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("q") {
		case "Lab Band":
			_, _ = w.Write([]byte(`{"data":[{"name":"Lab Band Tribute","picture_xl":"https://img.test/tribute.jpg"},
				{"name":"lab band","picture_big":"https://img.test/big.jpg","picture_xl":"https://img.test/lab-band.jpg"}]}`))
		case "No Picture":
			_, _ = w.Write([]byte(`{"data":[{"name":"No Picture","picture_xl":"https://img.test/images/artist//1000x1000-000000-80-0-0.jpg"}]}`))
		case "Elsewhere":
			_, _ = w.Write([]byte(`{"data":[{"name":"Somebody Else","picture_xl":"https://img.test/else.jpg"}]}`))
		default:
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestArtistImagesFindsTheSameName(t *testing.T) {
	var calls atomic.Int32
	srv := fakeDeezer(t, &calls)
	d := &ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond}
	ctx := context.Background()
	got, err := d.Image(ctx, "Lab Band")
	if err != nil || got != "https://img.test/lab-band.jpg" {
		t.Fatalf("image %q %v", got, err)
	}
	// Kept: a second ask makes no call.
	if got, _ := d.Image(ctx, "Lab Band"); got != "https://img.test/lab-band.jpg" || calls.Load() != 1 {
		t.Fatalf("cached %q calls %d", got, calls.Load())
	}
	for _, name := range []string{"No Picture", "Elsewhere", "Nobody"} {
		if got, err := d.Image(ctx, name); err != nil || got != "" {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
}

func TestArtistImagesDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	d := &ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond, Timeout: time.Second}
	if got, err := d.Image(context.Background(), "Lab Band"); err == nil || got != "" {
		t.Fatalf("down: %q %v", got, err)
	}
	var off *ArtistImages
	if got, err := off.Image(context.Background(), "Lab Band"); err != nil || got != "" {
		t.Fatalf("nil provider: %q %v", got, err)
	}
}

// TestArtistImagesLooseName: with no exact name match, a match after
// dropping case, accents, punctuation and a leading "The" counts.
func TestArtistImagesLooseName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"Beyonce Tribute","picture_xl":"https://img.test/no.jpg"},{"name":"The Beyoncé","picture_xl":"https://img.test/yes.jpg"}]}`))
	}))
	t.Cleanup(srv.Close)
	d := &ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond}
	if p, err := d.Image(context.Background(), "Beyonce"); err != nil || p != "https://img.test/yes.jpg" {
		t.Fatalf("%q %v", p, err)
	}
}
