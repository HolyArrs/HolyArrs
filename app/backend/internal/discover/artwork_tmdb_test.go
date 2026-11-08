package discover

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// toServer sends every request to srv, whatever host it names.
type toServer struct{ srv *url.URL }

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = t.srv.Scheme, t.srv.Host
	return http.DefaultTransport.RoundTrip(r)
}

// route: GET /api/v1/artwork/tmdb/{size}/{file}
func TestTMDBArtworkFetchesOnceAndCaches(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	var asked string
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		asked = r.URL.Path
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = io.WriteString(w, "JPEGBYTES")
	}))
	defer pics.Close()
	u, _ := url.Parse(pics.URL)
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{metadata.ImageBase + "/"}, HTTP: &http.Client{Transport: toServer{u}}}
	var misses ArtworkBudget
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/artwork/tmdb/{size}/{file}", func(w http.ResponseWriter, r *http.Request) { TMDBArtwork(w, r, art, &misses) })
	srv := httptest.NewServer(m)
	defer srv.Close()

	var etag string
	for i := 0; i < 2; i++ {
		res, err := http.Get(srv.URL + "/api/v1/artwork/tmdb/w342/poster1.jpg")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || string(b) != "JPEGBYTES" || res.Header.Get("Content-Type") != "image/jpeg" {
			t.Fatalf("answer %d %q %v", res.StatusCode, b, res.Header)
		}
		if res.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" {
			t.Fatalf("cache header %q", res.Header.Get("Cache-Control"))
		}
		etag = res.Header.Get("ETag")
	}
	if hits.Load() != 1 || asked != "/t/p/w342/poster1.jpg" {
		t.Fatalf("TMDB asked %d times for %s", hits.Load(), asked)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/artwork/tmdb/w342/poster1.jpg", nil)
	req.Header.Set("If-None-Match", etag)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if etag == "" || res.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidate: etag %q, status %d", etag, res.StatusCode)
	}
	for _, p := range []string{"/api/v1/artwork/tmdb/w999/a.jpg", "/api/v1/artwork/tmdb/w342/a.exe"} {
		if res, _ := http.Get(srv.URL + p); res.StatusCode != 404 {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
}
