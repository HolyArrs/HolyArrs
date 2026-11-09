package library

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// toLocal sends every request to srv (TMDB's host and the metadata
// server's alike), recording each request's host, path and Referer; down
// lists hosts that do not answer at all.
type toLocal struct {
	srv  *httptest.Server
	down map[string]bool
	mu   sync.Mutex
	seen []string
	refs []string
}

func (l *toLocal) RoundTrip(r *http.Request) (*http.Response, error) {
	l.mu.Lock()
	l.seen = append(l.seen, r.URL.Host+r.URL.Path)
	l.refs = append(l.refs, r.Header.Get("Referer"))
	l.mu.Unlock()
	if l.down[r.URL.Host] {
		return nil, errors.New("no route to host")
	}
	u, _ := url.Parse(l.srv.URL)
	c := r.Clone(r.Context())
	c.URL.Scheme, c.URL.Host, c.Host = u.Scheme, u.Host, r.URL.Host
	return http.DefaultTransport.RoundTrip(c)
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const server = "https://api.example.test"

// TestTMDBDownAsksTheServer: when TMDB's network does not answer (or
// answers server trouble) the picture comes through the metadata server's
// pass-through and is kept under TMDB's own address; a real 404 is never
// passed through.
func TestTMDBDownAsksTheServer(t *testing.T) {
	pic := jpegBytes(t, 20, 30)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "image.tmdb.org" && strings.HasSuffix(r.URL.Path, "/busy.jpg"):
			w.WriteHeader(http.StatusBadGateway)
		case r.Host == "image.tmdb.org":
			http.NotFound(w, r)
		case r.URL.Path == "/v1/img/tmdb/w500/abc.jpg", r.URL.Path == "/v1/img/tmdb/w780/busy.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(pic)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	tr := &toLocal{srv: srv, down: map[string]bool{"image.tmdb.org": true}}
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{metadata.ImageBase + "/", server + "/v1/img/"}, HTTP: &http.Client{Transport: tr}, Fallback: TMDBPassThrough(server)}
	src := metadata.ImageBase + "/w500/abc.jpg"
	name, err := c.Fetch(context.Background(), src)
	if err != nil || name != ArtworkName(src) {
		t.Fatalf("%q %v", name, err)
	}
	if b, _ := os.ReadFile(filepath.Join(c.Dir, name)); !bytes.Equal(b, pic) {
		t.Fatal("pass-through picture not kept")
	}
	if got := strings.Join(tr.seen, " "); got != "image.tmdb.org/t/p/w500/abc.jpg api.example.test/v1/img/tmdb/w500/abc.jpg" {
		t.Fatalf("asked %s", got)
	}
	// TMDB up but in trouble: the server again.
	tr.down = nil
	if _, err := c.Fetch(context.Background(), metadata.ImageBase+"/w780/busy.jpg"); err != nil {
		t.Fatalf("5xx not passed through: %v", err)
	}
	// TMDB says the picture is gone: no pass-through.
	tr.seen = nil
	_, err = c.Fetch(context.Background(), metadata.ImageBase+"/w500/gone.jpg")
	var se *ArtworkStatusError
	if !errors.As(err, &se) || se.Code != 404 || len(tr.seen) != 1 {
		t.Fatalf("a 404 was passed through: %v %v", err, tr.seen)
	}
	if TMDBPassThrough(server)(metadata.ImageBase+"/w500/../x.jpg") != "" || TMDBPassThrough(server)("https://evil.test/t/p/w500/a.jpg") != "" {
		t.Fatal("pass-through for an odd address")
	}
}

// TestNoRefererOnRedirect: a picture that redirects is fetched without a
// Referer (Go would add one on the second hop).
func TestNoRefererOnRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/a.jpg") {
			http.Redirect(w, r, metadata.ImageBase+"/w500/b.jpg", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer srv.Close()
	tr := &toLocal{srv: srv}
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{metadata.ImageBase + "/"}, HTTP: &http.Client{Transport: tr}}
	if _, err := c.Fetch(context.Background(), metadata.ImageBase+"/w500/a.jpg"); err != nil {
		t.Fatal(err)
	}
	if len(tr.refs) != 2 || tr.refs[0] != "" || tr.refs[1] != "" {
		t.Fatalf("Referer sent: %q", tr.refs)
	}
}

// TestTMDBCopiesKeptSixMonths: a TMDB copy records when it was fetched; past
// TMDBKeep the sweep fetches it again when it is still shown, and drops it
// when nobody has shown it for a month. A copy of unknown age (stored
// before ages were kept) is fetched again the next time it is shown.
func TestTMDBCopiesKeptSixMonths(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked++
		mu.Unlock()
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	defer srv.Close()
	c := &ArtworkCache{Dir: t.TempDir(), Allowed: []string{metadata.ImageBase + "/"}, HTTP: &http.Client{Transport: &toLocal{srv: srv}}}
	src := metadata.ImageBase + "/w342/p.jpg"
	name, err := c.Fetch(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(c.Dir, tmdbName(name))
	pic := filepath.Join(c.Dir, name)
	if b, _ := os.ReadFile(mark); string(b) != src {
		t.Fatalf("mark %q", b)
	}
	old := time.Now().Add(-TMDBKeep - time.Hour)
	_ = os.Chtimes(mark, old, old)
	c.SweepTMDB()
	if st, _ := os.Stat(mark); asked != 2 || time.Since(st.ModTime()) > time.Minute {
		t.Fatalf("an old copy still shown was not fetched again (%d asks)", asked)
	}
	// Old and unshown for over a month: dropped.
	unused := time.Now().Add(-tmdbIdle - time.Hour)
	_ = os.Chtimes(mark, old, old)
	_ = os.Chtimes(pic, unused, unused)
	c.SweepTMDB()
	if _, err := os.Stat(pic); !os.IsNotExist(err) || asked != 2 {
		t.Fatalf("an old unshown copy was kept (%v, %d asks)", err, asked)
	}
	if _, err := os.Stat(mark); !os.IsNotExist(err) {
		t.Fatal("mark left behind")
	}
	// A copy from before ages were kept: fetched again when shown.
	if _, err := c.Fetch(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(mark)
	if _, err := c.Fetch(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(mark); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a copy of unknown age was not fetched again")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
