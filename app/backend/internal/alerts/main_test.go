package alerts

import (
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/feature"
)

// TestMain runs this package with the held-back features switched on
// (internal/feature); the switched-off side is tested in internal/feature.
func TestMain(m *testing.M) {
	os.Setenv("HOLYARRS_FLAGS", strings.Join(feature.Held, ","))
	os.Exit(m.Run())
}
