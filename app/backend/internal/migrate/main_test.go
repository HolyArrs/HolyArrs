package migrate

import (
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// TestMain runs this package with the held-back features switched on
// (internal/feature); the switched-off side is tested in internal/feature.
func TestMain(m *testing.M) {
	startup.RegisterHeldFeatures()
	os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	os.Exit(m.Run())
}
