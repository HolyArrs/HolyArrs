package auth

import (
	"os"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/feature"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/routehits"
)

// TestMain fails a full run of this package when a route it registers was
// never served to a test request.
func TestMain(m *testing.M) {
	// Held-back features are tested switched on (internal/feature).
	os.Setenv("HOLYARRS_FLAGS", strings.Join(feature.Held, ","))

	os.Exit(routehits.Check(m.Run(), "."))
}
