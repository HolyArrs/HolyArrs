package accounts

import (
	"os"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"testing"
)

// TestMain fails a full run of this package when a route it registers was
// never served to a test request.
func TestMain(m *testing.M) {
	// Held-back features are tested switched on (internal/feature).
	startup.RegisterHeldFeatures()
	os.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))

	os.Exit(webserver.CheckRouteHits(m.Run(), "."))
}
