package discover

import (
	"bytes"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// route: GET /api/v1/artwork/{name}
// ?size=250 answers the stored Library picture's small copy (for thumbnails); without it, the picture as stored.
func TestArtworkSmallCopy(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	art := &library.ArtworkCache{Dir: filepath.Join(t.TempDir(), "art")}
	if err := os.MkdirAll(art.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1000, 1500)), nil); err != nil {
		t.Fatal(err)
	}
	name := "0123456789abcdef0123456789abcdef.jpg"
	if err := os.WriteFile(filepath.Join(art.Dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Artwork: art}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/artwork/{name}", func(w http.ResponseWriter, r *http.Request) { LibraryArtwork(w, r, lib.Artwork) })
	srv := httptest.NewServer(m)
	defer srv.Close()
	get := func(path string) (int, int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, err, resp)
		}
		defer resp.Body.Close()
		cfg, _, err := image.DecodeConfig(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Width, cfg.Height, resp.Header.Get("Cache-Control")
	}
	if w, h, _ := get("/api/v1/artwork/" + name); w != 1000 || h != 1500 {
		t.Fatalf("stored %dx%d", w, h)
	}
	if w, h, cc := get("/api/v1/artwork/" + name + "?size=250"); max(w, h) != 250 || cc != "private, max-age=86400" {
		t.Fatalf("small %dx%d %q", w, h, cc)
	}
}
