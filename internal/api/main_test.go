package api

import (
	"os"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/routehits"
	"github.com/CoderLuii/holyarrs/internal/youtube"
)

// TestMain fails a full run of this package when a route it registers was
// never served to a test request.
func TestMain(m *testing.M) {
	wrapMux = routehits.Wrap
	// The music cover preload runs only in the tests that ask for it.
	musicPreloadOn = false
	// YouTube itself is never asked: a test that looks a video up points
	// the reader at a fake (fakeReader); any other lookup fails at once.
	trailerReader = &youtube.Reader{Base: "http://127.0.0.1:1"}
	os.Exit(routehits.Check(m.Run(), "."))
}
