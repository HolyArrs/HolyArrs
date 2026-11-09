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

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakePictures is a fake metadata server picture store holding the lab
// artist's photo; Deezer-style addresses (an artist by id, a name search
// finding an impostor) are there too and must never be asked.
// asked, when set, counts the requests.
func fakePictures(t *testing.T, down *atomic.Bool, asked ...*atomic.Int32) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, n := range asked {
			n.Add(1)
		}
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/v1/img/artist/" + labArtist + "/500.webp":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("ÿØÿà" + strings.Repeat("s", 200)))
		case "/search/artist":
			// Another artist with the same name: never used.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":99,"name":"Lab Band","picture_xl":"` + srv.URL + `/pic/impostor.jpg"}]}`))
		case "/artist/27":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":27,"name":"Lab Band","picture_xl":"` + srv.URL + `/pic/lab-band.jpg"}`))
		case "/pic/lab-band.jpg", "/pic/impostor.jpg":
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
	svc.ImageServer = srv.URL + "/v1/img"
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{srv.URL + "/"}, HTTP: &http.Client{}}
}

func artworkOf(t *testing.T, s *database.Store, id int64) []string {
	t.Helper()
	var ti mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", id, &ti); err != nil {
		t.Fatal(err)
	}
	return ti.Artwork
}

// TestArtistImageOnAdd: adding an artist asks for no picture itself; the
// refresh queued with the add finds it.
func TestArtistImageOnAdd(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	var down atomic.Bool
	var asked atomic.Int32
	withPictures(t, svc, fakePictures(t, &down, &asked))
	id, jobID, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("the add asked for a picture itself: %d requests", n)
	}
	var j domain.Job
	if err := database.Get(ctx, s.Read(), "jobs", jobID, &j); err != nil || j.Kind != KindRefreshTitle {
		t.Fatalf("queued job %+v %v", j, err)
	}
	if err := svc.handleRefreshTitle(ctx, &jobs.Run{Job: j}); err != nil {
		t.Fatal(err)
	}
	art := artworkOf(t, s, id)
	if len(art) != 1 || !strings.HasPrefix(art[0], "poster:") {
		t.Fatalf("artwork %v", art)
	}
	if want := "poster:" + ArtworkName(svc.ImageServer+"/artist/"+labArtist+"/500.webp"); art[0] != want {
		t.Fatalf("artwork %v, want the server's stored photo %s", art, want)
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
