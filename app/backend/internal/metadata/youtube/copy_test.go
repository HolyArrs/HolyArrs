package youtube

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The metadata server is its own Go module and cannot import this package,
// so it keeps a copy. A change to the reader (a moved door) is made here
// and copied there: this test fails until both are the same.
func TestMetadataServerCopyIsTheSame(t *testing.T) {
	for _, name := range []string{"youtube.go", "youtube_test.go"} {
		here, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		there, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "services", "metadata", "internal", "youtube", name))
		if err != nil {
			t.Fatal(err)
		}
		// Line endings do not count (a Windows checkout may change them).
		lf := func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
		if !bytes.Equal(lf(here), lf(there)) {
			t.Fatalf("services/metadata/internal/youtube/%s differs from internal/youtube/%s: copy it over", name, name)
		}
	}
}
