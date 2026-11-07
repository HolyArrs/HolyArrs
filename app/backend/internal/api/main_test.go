package api

import (
	"os"
	"strings"

	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

// TestMain fails a full run of this package when a route it registers was
// never served to a test request.
func TestMain(m *testing.M) {
	// Held-back features are tested switched on (internal/feature).
	os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	wrapMux = webserver.RecordRouteHits
	// The music cover preload runs only in the tests that ask for it.
	musicPreloadOn = false
	// YouTube itself is never asked: a test that looks a video up points
	// the reader at a fake (fakeReader); any other lookup fails at once.
	trailerReader = &youtube.Reader{Base: "http://127.0.0.1:1"}
	os.Exit(webserver.CheckRouteHits(m.Run(), "."))
}
