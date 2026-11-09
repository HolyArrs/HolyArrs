package library

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmallerMakesASmallCopy: 250 is made here from the stored 500 (once),
// keeps the shape, counts toward the cap, and a picture already that small
// is used as it is.
func TestSmallerMakesASmallCopy(t *testing.T) {
	dir := t.TempDir()
	c := &ArtworkCache{Dir: dir}
	big := ArtworkName("https://api.example.test/v1/img/album/x/500.webp?v=1")
	if err := os.WriteFile(filepath.Join(dir, big), jpegBytes(t, 500, 400), 0o640); err != nil {
		t.Fatal(err)
	}
	name, p, err := c.Smaller(big, 250)
	if err != nil || name == big || !strings.HasSuffix(name, ".jpg") {
		t.Fatalf("%q %v", name, err)
	}
	f, _ := os.Open(p)
	cfg, err := jpeg.DecodeConfig(f)
	_ = f.Close()
	if err != nil || cfg.Width != 250 || cfg.Height != 200 {
		t.Fatalf("made %dx%d (%v)", cfg.Width, cfg.Height, err)
	}
	st, _ := os.Stat(p)
	again, _, err := c.Smaller(big, 250)
	if st2, _ := os.Stat(p); err != nil || again != name || st2.Size() != st.Size() {
		t.Fatalf("not reused: %q %v", again, err)
	}
	if c.size < st.Size() {
		t.Fatal("the copy was not counted toward the cap")
	}
	// A small PNG stays as it is.
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 200, 200)))
	tiny := ArtworkName("https://api.example.test/v1/img/album/y/500.webp")
	_ = os.WriteFile(filepath.Join(dir, tiny), b.Bytes(), 0o640)
	if n, _, err := c.Smaller(tiny, 250); err != nil || n != tiny {
		t.Fatalf("small picture copied: %q %v", n, err)
	}
	if _, _, err := c.Smaller("0123456789abcdef0123456789abcdef.webp", 250); !errors.Is(err, ErrArtworkNotStored) {
		t.Fatalf("missing picture: %v", err)
	}
}
