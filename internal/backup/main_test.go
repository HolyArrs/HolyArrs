package backup

import (
	"os"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/routehits"
)

// TestMain fails a full run of this package when a route it registers was
// never served to a test request.
func TestMain(m *testing.M) {

	os.Exit(routehits.Check(m.Run(), "."))
}
