package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// openTest opens a fresh database in a temporary folder.
func openTest(t *testing.T, opts database.Options) (*database.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}
