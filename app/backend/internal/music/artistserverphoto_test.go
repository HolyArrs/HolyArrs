package music

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestArtistServerPhotoFirst: the metadata server's sharp artist photo wins
// over the chart's picture; a placeholder answer is refused, so the old
// sources still run.
func TestArtistServerPhotoFirst(t *testing.T) {
	t.Parallel()
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	const sharp, cold = "9fff2f8a-21e6-47de-a2b8-7f449929d43f", "44444444-4444-4444-8444-444444444444"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		switch r.URL.Path {
		case "/v1/img/artist/" + sharp + "/500.webp":
			_, _ = w.Write([]byte("\xff\xd8\xff sharp"))
		case "/v1/img/artist/" + cold + "/500.webp":
			w.Header().Set("X-Metadata-Placeholder", "true")
			_, _ = w.Write([]byte("\xff\xd8\xff placeholder"))
		default:
			_, _ = w.Write([]byte("\xff\xd8\xff chart " + r.URL.Path))
		}
	}))
	defer srv.Close()
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: srv.Client()}
	lib := &library.Service{Store: s, Artwork: art}
	a := New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: lib, ImageBase: srv.URL + "/v1/img", Version: "test"})
	a.music.mu.Lock()
	a.music.names = map[string]metadata.MusicTrendItem{
		"artist|" + sharp: {MBID: sharp, Image: srv.URL + "/chart/blurry.jpg"},
		"artist|" + cold:  {MBID: cold, Image: srv.URL + "/chart/cold.jpg"},
	}
	a.music.mu.Unlock()
	ctx := context.Background()
	if name, _, ok := a.musicTrendArtFile(ctx, "artist", sharp, "500"); !ok || name != library.ArtworkName(srv.URL+"/v1/img/artist/"+sharp+"/500.webp") {
		t.Fatalf("sharp artist picture %q %v, want the server photo", name, ok)
	}
	if name, _, ok := a.musicTrendArtFile(ctx, "artist", cold, "500"); ok {
		t.Fatalf("cold artist picture %q, want none: pictures come only from the server", name)
	}
	if _, ok := art.Path(library.ArtworkName(srv.URL + "/v1/img/artist/" + cold + "/500.webp")); ok {
		t.Fatal("a placeholder was stored as the artist photo")
	}
}
