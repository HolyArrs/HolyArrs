package frontend

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

// The server serves Files at /. It must be rooted inside dist, not at it.
func TestFilesRootedAtDist(t *testing.T) {
	if Files == nil {
		t.Fatal("fs.Sub failed: Files is nil")
	}
	if _, err := fs.Stat(Files, ".gitkeep"); err != nil {
		if _, err2 := fs.Stat(Files, "index.html"); err2 != nil {
			t.Fatalf("Files not rooted at dist (no .gitkeep or index.html): %v", err)
		}
	}
	if _, err := fs.Stat(Files, "dist"); err == nil {
		t.Fatal("Files still contains a dist/ prefix")
	}
}

// A file kept only as a brotli copy opens under its plain name, unpacked.
func TestPackedOpensBrotliCopy(t *testing.T) {
	var z bytes.Buffer
	w := brotli.NewWriter(&z)
	_, _ = w.Write([]byte("hello pages"))
	_ = w.Close()
	p := packed{fstest.MapFS{"a.js.br": {Data: z.Bytes()}, "b.css": {Data: []byte("plain")}}}
	for name, want := range map[string]string{"a.js": "hello pages", "b.css": "plain"} {
		b, err := fs.ReadFile(p, name)
		if err != nil || string(b) != want {
			t.Fatalf("%s = %q, %v; want %q", name, b, err, want)
		}
	}
	if st, err := fs.Stat(p, "a.js"); err != nil || st.Size() != 11 || st.IsDir() {
		t.Fatalf("stat a.js = %v, %v", st, err)
	}
	if b, err := fs.ReadFile(p, "a.js.br"); err != nil || !bytes.Equal(b, z.Bytes()) {
		t.Fatalf("a.js.br is not the stored copy: %v", err)
	}
	if _, err := fs.Stat(p, "none.js"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("none.js: %v", err)
	}
}
