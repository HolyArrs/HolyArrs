package backups

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"os"
	"testing"
)

// TestMain fails a full run of this package when a route it registers was
// never served to a test request.
func TestMain(m *testing.M) {

	os.Exit(webserver.CheckRouteHits(m.Run(), "."))
}
