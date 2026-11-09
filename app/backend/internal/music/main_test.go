package music

import (
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestMain(m *testing.M) {
	// Held-back features are tested switched on (internal/feature).
	startup.RegisterHeldFeatures()
	parts.Register()
	os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	// The music cover preload runs only in the tests that ask for it.
	musicPreloadOn = false
	// YouTube itself is never asked.
	metadata.UseReader(&metadata.VideoReader{Base: "http://127.0.0.1:1"})
	os.Exit(m.Run())
}
