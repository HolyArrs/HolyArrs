package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// fakePictures is a fake Deezer artist search that also serves the picture.
func fakePictures(t *testing.T, down *atomic.Bool) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/search/artist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"Lab Band","picture_xl":"` + srv.URL + `/pic/lab-band.jpg"}]}`))
		case "/pic/lab-band.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff\xe0" + strings.Repeat("x", 200)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withPictures(t *testing.T, svc *Service, srv *httptest.Server) {
	svc.ArtistImages = &metadata.ArtistImages{BaseURL: srv.URL, Gap: time.Millisecond, Timeout: time.Second}
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/pic/"}}
}

func artworkOf(t *testing.T, s *store.Store, id int64) []string {
	t.Helper()
	var ti domain.Title
	if err := store.Get(context.Background(), s.Read(), "titles", id, &ti); err != nil {
		t.Fatal(err)
	}
	return ti.Artwork
}

func TestArtistImageOnAdd(t *testing.T) {
	svc, s, _, _ := musicService(t)
	var down atomic.Bool
	withPictures(t, svc, fakePictures(t, &down))
	id, _, err := svc.AddArtist(context.Background(), labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	art := artworkOf(t, s, id)
	if len(art) != 1 || !strings.HasPrefix(art[0], "poster:") {
		t.Fatalf("artwork %v", art)
	}
}

func TestArtistRefreshBackfillsImage(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	id, _, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if art := artworkOf(t, s, id); len(art) != 0 {
		t.Fatalf("no provider yet, artwork %v", art)
	}
	var down atomic.Bool
	withPictures(t, svc, fakePictures(t, &down))
	if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if art := artworkOf(t, s, id); len(art) != 1 || !strings.HasPrefix(art[0], "poster:") {
		t.Fatalf("backfilled artwork %v", art)
	}
}

func TestArtistImageProviderDownIsQuiet(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	var down atomic.Bool
	down.Store(true)
	withPictures(t, svc, fakePictures(t, &down))
	id, _, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatalf("add with the picture source down: %v", err)
	}
	if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatalf("refresh with the picture source down: %v", err)
	}
	if art := artworkOf(t, s, id); len(art) != 0 {
		t.Fatalf("artwork %v", art)
	}
}
