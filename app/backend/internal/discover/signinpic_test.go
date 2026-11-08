package discover

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/public/signin-pic/t/p/{size}/{file}
// The sign-in answer holds same-origin addresses at served sizes, those
// pictures load through the artwork cache, and anything else is refused.
func TestSigninPicServesOnlyItsOwnArt(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = io.WriteString(w, "JPEGBYTES")
	}))
	defer pics.Close()
	u, _ := url.Parse(pics.URL)
	a, _ := signinRig(t, [][2]string{{"movie", ""}})
	cache := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{metadata.ImageBase + "/"}, HTTP: &http.Client{Transport: toServer{u}}}
	a.Artwork = func() *library.ArtworkCache { return cache }

	w, shots := getSigninArt(t, a, "10.0.0.9")
	if w.Code != 200 || len(shots) == 0 {
		t.Fatalf("code %d, %d shots", w.Code, len(shots))
	}
	s := shots[0]
	for _, x := range []string{s.Backdrop.URL, s.Logo, s.Poster.URL} {
		if !strings.HasPrefix(x, "/api/v1/public/signin-pic/t/p/") {
			t.Fatalf("not same-origin: %s", x)
		}
	}
	if !strings.Contains(s.Backdrop.URL, "/w1280/") || !strings.Contains(s.Poster.URL, "/w780/") {
		t.Fatalf("sizes: %s %s", s.Backdrop.URL, s.Poster.URL)
	}

	get := func(p string) int {
		r := httptest.NewRequest("GET", p, nil)
		r.RemoteAddr = "10.0.0.9:5555"
		rec := httptest.NewRecorder()
		signinMux(a).ServeHTTP(rec, r)
		return rec.Code
	}
	if c := get(s.Backdrop.URL); c != 200 {
		t.Fatalf("own backdrop: %d", c)
	}
	if c := get(s.Logo); c != 200 {
		t.Fatalf("own logo: %d", c)
	}
	if c := get("/api/v1/public/signin-pic/t/p/" + "original/" + filepath.Base(s.Backdrop.URL)); c != 200 {
		t.Fatalf("own backdrop at original: %d", c)
	}
	if c := get("/api/v1/public/signin-pic/t/p/" + "original/" + filepath.Base(s.Poster.URL)); c != 200 {
		t.Fatalf("own poster at original: %d", c)
	}
	before := hits.Load()
	for _, p := range []string{
		"/api/v1/public/signin-pic/t/p/" + "w1280/notlisted.jpg",
		"/api/v1/public/signin-pic/t/p/" + "original/" + filepath.Base(s.Logo),
		"/api/v1/public/signin-pic/t/p/" + "w3840/" + filepath.Base(s.Backdrop.URL),
		"/api/v1/public/signin-pic/t/p/" + "w1280/" + url.PathEscape("https://evil.example/x.jpg"),
		"/api/v1/public/signin-pic/t/p/" + "w1280/..%2f..%2fetc%2fpasswd",
		"/api/v1/public/signin-pic/http://169.254.169.254/latest/x.jpg",
	} {
		if c := get(p); c == 200 {
			t.Fatalf("%s served", p)
		}
	}
	if hits.Load() != before {
		t.Fatal("a refused address reached the picture host")
	}
}
