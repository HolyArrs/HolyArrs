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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// pictureWorld is one fake server for TheAudioDB, Deezer and the Cover Art
// Archive, each switchable, that records the order of the asks.
type pictureWorld struct {
	srv                  *httptest.Server
	audioDB, deezer, caa bool
	key                  string // TheAudioDB key; "" is off
	mu                   sync.Mutex
	asked                []string
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
	w := &pictureWorld{key: "own-key"}
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/adb/"):
			w.log("audiodb")
			rw.Header().Set("Content-Type", "application/json")
			if !w.audioDB {
				_, _ = rw.Write([]byte(`{"artists":null}`))
				return
			}
			_, _ = rw.Write([]byte(`{"artists":[{"strArtist":"Lab Band","strMusicBrainzID":"` + labArtist + `","strArtistThumb":"` + w.srv.URL +
				`/pic/adb-thumb.jpg","strArtistFanart":"https://img.test/fanart.jpg","strArtistLogo":"https://img.test/logo.png"}]}`))
		case p == "/search/artist":
			w.log("deezer")
			rw.Header().Set("Content-Type", "application/json")
			if !w.deezer {
				_, _ = rw.Write([]byte(`{"data":[]}`))
				return
			}
			_, _ = rw.Write([]byte(`{"data":[{"name":"Lab Band","picture_xl":"` + w.srv.URL + `/pic/deezer.jpg"}]}`))
		case strings.HasPrefix(p, "/caa/release-group/"):
			w.log("caa:" + strings.TrimSuffix(strings.TrimPrefix(p, "/caa/release-group/"), "/front-500"))
			if !w.caa {
				http.NotFound(rw, r)
				return
			}
			http.Redirect(rw, r, w.srv.URL+"/pic/cover.jpg", http.StatusTemporaryRedirect)
		case strings.HasPrefix(p, "/pic/"):
			rw.Header().Set("Content-Type", "image/jpeg")
			_, _ = rw.Write([]byte("\xff\xd8\xff\xe0" + strings.Repeat("x", 200) + p))
		default:
			http.NotFound(rw, r)
		}
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func withPictureWorld(t *testing.T, svc *Service, w *pictureWorld) {
	svc.AudioDB = &metadata.AudioDB{BaseURL: w.srv.URL + "/adb", Gap: time.Millisecond, Timeout: time.Second,
		Key: func(context.Context) (string, error) { return w.key, nil }}
	svc.ArtistImages = &metadata.ArtistImages{BaseURL: w.srv.URL, Gap: time.Millisecond, Timeout: time.Second}
	svc.CoverArtBase = w.srv.URL + "/caa"
	svc.Artwork = &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{w.srv.URL + "/pic/", w.srv.URL + "/caa/"}, HTTP: &http.Client{}}
}

func titleOf(t *testing.T, s *store.Store, id int64) domain.Title {
	t.Helper()
	var ti domain.Title
	if err := store.Get(context.Background(), s.Read(), "titles", id, &ti); err != nil {
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

// TestArtistPicturesOrder: TheAudioDB is asked first and, when it has the
// artist, fills the poster, backdrop and logo; Deezer comes next; the
// earliest studio album's cover is the last resort.
func TestArtistPicturesOrder(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name                 string
		audioDB, deezer, caa bool
		source, order        string
		wide                 bool
	}{
		{"audiodb", true, true, true, "theaudiodb", "audiodb", true},
		{"deezer", false, true, true, "deezer", "audiodb,audiodb,deezer", false},
		{"cover", false, false, true, "coverartarchive", "audiodb,audiodb,deezer,caa:" + rgOld, false},
		{"none", false, false, false, "", "audiodb,audiodb,deezer,caa:" + rgOld, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, s, _, _ := musicService(t)
			id := addBareArtist(t, svc)
			w := newPictureWorld(t)
			w.audioDB, w.deezer, w.caa = tc.audioDB, tc.deezer, tc.caa
			withPictureWorld(t, svc, w)
			src, err := svc.FillArtistPictures(ctx, id, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			if src != tc.source {
				t.Fatalf("source %q, want %q (asked %s)", src, tc.source, w.order())
			}
			if !strings.HasPrefix(w.order(), tc.order) {
				t.Fatalf("asked %s, want %s", w.order(), tc.order)
			}
			ti := titleOf(t, s, id)
			if (src != "") != (len(ti.Artwork) == 1 && strings.HasPrefix(ti.Artwork[0], "poster:")) {
				t.Fatalf("artwork %v", ti.Artwork)
			}
			if tc.wide && (ti.Backdrop != "https://img.test/fanart.jpg" || ti.Logo != "https://img.test/logo.png") {
				t.Fatalf("backdrop %q logo %q", ti.Backdrop, ti.Logo)
			}
			if !tc.wide && (ti.Backdrop != "" || ti.Logo != "") {
				t.Fatalf("only TheAudioDB fills the backdrop and logo: %q %q", ti.Backdrop, ti.Logo)
			}
		})
	}
}

// TestArtistPicturesNoKey: with no TheAudioDB key it is never asked; the
// order is Deezer, then the album cover.
func TestArtistPicturesNoKey(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		deezer       bool
		source, want string
	}{{true, "deezer", "deezer"}, {false, "coverartarchive", "deezer,caa:" + rgOld}} {
		svc, s, _, _ := musicService(t)
		id := addBareArtist(t, svc)
		w := newPictureWorld(t)
		w.key, w.audioDB, w.deezer, w.caa = "", true, tc.deezer, true
		withPictureWorld(t, svc, w)
		src, err := svc.FillArtistPictures(ctx, id, domain.System)
		if err != nil || src != tc.source || w.order() != tc.want {
			t.Fatalf("source %q %v, asked %s, want %s", src, err, w.order(), tc.want)
		}
		if ti := titleOf(t, s, id); ti.Backdrop != "" || ti.Logo != "" {
			t.Fatalf("backdrop %q logo %q", ti.Backdrop, ti.Logo)
		}
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
	w.audioDB, w.deezer = true, true
	withPictureWorld(t, svc, w)
	if _, err := svc.FillArtistPictures(ctx, id, domain.System); err != nil {
		t.Fatal(err)
	}
	ti := titleOf(t, s, id)
	if len(ti.Artwork) != 1 || ti.Artwork[0] != "poster:mine.jpg" || ti.Logo != "https://mine.test/logo.png" || ti.Backdrop != "https://img.test/fanart.jpg" {
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
	w.audioDB = true
	withPictureWorld(t, svc, w)
	id := addBareArtist(t, svc)
	if w.order() != "" {
		t.Fatalf("the add asked %s itself", w.order())
	}
	js, err := store.Select[domain.Job](ctx, s.Read(), "jobs", "kind = ?", KindArtistPictures)
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
	w.deezer = true
	withPictureWorld(t, svc, w)
	res, err := svc.FillAllArtistPictures(ctx, domain.System, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ti := titleOf(t, s, id); len(ti.Artwork) != 1 {
		t.Fatalf("artwork %v", ti.Artwork)
	}
	if res.Filled != 1 || res.Sources["deezer"] != 1 || res.By[id] != "deezer" {
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
