package music

import (
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/features"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

func TestMain(m *testing.M) {
	// Held-back features are tested switched on (internal/feature).
	features.Register()
	parts.Register()
	os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	// The music cover preload runs only in the tests that ask for it.
	musicPreloadOn = false
	// YouTube itself is never asked.
	metadata.UseReader(&youtube.Reader{Base: "http://127.0.0.1:1"})
	os.Exit(m.Run())
}
