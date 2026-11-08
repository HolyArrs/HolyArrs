package requests

import (
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// TestMain turns on the held-back features, since Trakt lists need one. The
// switched-off side is tested in the feature package.
func TestMain(m *testing.M) {
	startup.RegisterHeldFeatures()
	os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	os.Exit(m.Run())
}
