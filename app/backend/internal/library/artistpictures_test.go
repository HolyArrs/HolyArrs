package library

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// pictureWorld is a fake metadata server picture store (photo, wide,
// logo, each switchable) that records every ask; any other address (an
// outside source) is recorded too and must never be asked.
type pictureWorld struct {
	srv               *httptest.Server
	photo, wide, logo bool
	mu                sync.Mutex
	asked             []string
}

func (w *pictureWorld) log(s string) {
	w.mu.Lock()
	w.asked = append(w.asked, s)
	w.mu.Unlock()
}

func (w *pictureWorld) order() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.asked, ",")
}

func newPictureWorld(t *testing.T) *pictureWorld {
	w := &pictureWorld{}
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		for _, k := range []struct {
			kind, file string
			on         bool
		}{{"artist", "500.webp", w.photo}, {"wide", "1920.webp", w.wide}, {"logo", "1200.png", w.logo}} {
			if p == "/v1/img/"+k.kind+"/"+labArtist+"/"+k.file {
				w.log(k.kind)
				if !k.on {
					http.NotFound(rw, r)
					return
				}
				rw.Header().Set("Content-Type", "image/jpeg")
				_, _ = rw.Write([]byte("\xff\xd8\xff\xe0" + strings.Repeat("x", 200) + p))
				return
			}
		}
		w.log("outside:" + p)
		http.NotFound(rw, r)
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func withPictureWorld(t *testing.T, svc *Service, w *pictureWorld) {
	svc.ImageServer = w.srv.URL + "/v1/img"
	svc.ArtistImages = &metadata.ArtistImages{BaseURL: w.srv.URL, Gap: time.Millisecond, Timeout: time.Second}
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{w.srv.URL + "/"}, HTTP: &http.Client{}}
}

func titleOf(t *testing.T, s *database.Store, id int64) mediafiles.Title {
	t.Helper()
	var ti mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", id, &ti); err != nil {
		t.Fatal(err)
	}
	return ti
}

func addBareArtist(t *testing.T, svc *Service) int64 {
	t.Helper()
	id, _, err := svc.AddArtist(context.Background(), labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestArtistPicturesFromServerOnly: the artist's photo, wide background
// and logo come only from the metadata server; one it has not stored yet
// fills nothing, and no outside source is ever asked.
func TestArtistPicturesFromServerOnly(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name              string
		photo, wide, logo bool
		source            string
	}{
		{"all", true, true, true, SourceServer},
		{"photo only", true, false, false, SourceServer},
		{"none stored", false, false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, s, _, _ := musicService(t)
			id := addBareArtist(t, svc)
			w := newPictureWorld(t)
			w.photo, w.wide, w.logo = tc.photo, tc.wide, tc.logo
			withPictureWorld(t, svc, w)
			src, err := svc.FillArtistPictures(ctx, id, domain.System)
			if err != nil || src != tc.source {
				t.Fatalf("source %q %v (asked %s)", src, err, w.order())
			}
			if strings.Contains(w.order(), "outside") {
				t.Fatalf("an outside source was asked: %s", w.order())
			}
			ti := titleOf(t, s, id)
			if tc.photo != (len(ti.Artwork) == 1 && strings.HasPrefix(ti.Artwork[0], "poster:")) {
				t.Fatalf("artwork %v", ti.Artwork)
			}
			if tc.wide != strings.HasPrefix(ti.Backdrop, "/api/v1/artwork/") || tc.logo != strings.HasPrefix(ti.Logo, "/api/v1/artwork/") {
				t.Fatalf("backdrop %q logo %q", ti.Backdrop, ti.Logo)
			}
		})
	}
}

// TestArtistPicturesKeepWhatIsThere: artwork or a logo already set is
// never replaced; an artist with every picture is not asked about.
func TestArtistPicturesKeepWhatIsThere(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	id := addBareArtist(t, svc)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET artwork = '["poster:mine.jpg"]', logo = 'https://mine.test/logo.png' WHERE id = ?`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w := newPictureWorld(t)
	w.photo, w.wide, w.logo = true, true, true
	withPictureWorld(t, svc, w)
	if _, err := svc.FillArtistPictures(ctx, id, domain.System); err != nil {
		t.Fatal(err)
	}
	ti := titleOf(t, s, id)
	if len(ti.Artwork) != 1 || ti.Artwork[0] != "poster:mine.jpg" || ti.Logo != "https://mine.test/logo.png" || !strings.HasPrefix(ti.Backdrop, "/api/v1/artwork/") {
		t.Fatalf("artwork %v logo %q backdrop %q", ti.Artwork, ti.Logo, ti.Backdrop)
	}
	n := len(w.order())
	if _, err := svc.FillArtistPictures(ctx, id, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.order()) != n {
		t.Fatalf("an artist with every picture was asked about again: %s", w.order())
	}
}

// TestArtistPicturesQueuedOnAdd: adding an artist queues its picture Job
// (the add itself asks nothing), and that Job fills the picture with no
// MusicBrainz refresh.
func TestArtistPicturesQueuedOnAdd(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	ctx := context.Background()
	w := newPictureWorld(t)
	w.photo = true
	withPictureWorld(t, svc, w)
	id := addBareArtist(t, svc)
	if w.order() != "" {
		t.Fatalf("the add asked %s itself", w.order())
	}
	js, err := database.Select[domain.Job](ctx, s.Read(), "jobs", "kind = ?", KindArtistPictures)
	if err != nil || len(js) != 1 || js[0].Subject == nil || js[0].Subject.ID != id {
		t.Fatalf("picture jobs %+v %v", js, err)
	}
	fm.artists = nil // MusicBrainz knows nothing now; the picture does not need it
	if err := svc.handleArtistPictures(ctx, &jobs.Run{Job: js[0]}); err != nil {
		t.Fatal(err)
	}
	if ti := titleOf(t, s, id); len(ti.Artwork) != 1 {
		t.Fatalf("artwork %v", ti.Artwork)
	}
}

// TestArtistPicturesAllMissing: with no subject every artist missing a
// picture is filled, and the result says which source filled each.
func TestArtistPicturesAllMissing(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	id := addBareArtist(t, svc)
	w := newPictureWorld(t)
	w.photo = true
	withPictureWorld(t, svc, w)
	res, err := svc.FillAllArtistPictures(ctx, domain.System, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ti := titleOf(t, s, id); len(ti.Artwork) != 1 {
		t.Fatalf("artwork %v", ti.Artwork)
	}
	if res.Filled != 1 || res.Sources[SourceServer] != 1 || res.By[id] != SourceServer {
		t.Fatalf("result %+v", res)
	}
}

// TestArtworkWildHost: "https://*.archive.org/" allows covers on any
// archive.org subdomain over https, and nothing else.
func TestArtworkWildHost(t *testing.T) {
	c := &ArtworkCache{Allowed: []string{"https://*.archive.org/"}}
	for u, want := range map[string]bool{
		"https://ia800.us.archive.org/1/items/x/front.jpg": true,
		"https://archive.org.evil.test/x.jpg":              false,
		"http://ia800.us.archive.org/x.jpg":                false,
		"https://evilarchive.org/x.jpg":                    false,
		"https://ia800.us.archive.org:444/x.jpg":           false,
	} {
		if got := c.allowed(u); got != want {
			t.Errorf("%s: %v", u, got)
		}
	}
}
