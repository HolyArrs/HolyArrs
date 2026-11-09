package dbtests

import (
	"io/fs"
	"testing"
	"testing/fstest"

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

// upTo is the main migration set up to and including version v.
func upTo(t *testing.T, v int) fs.FS {
	t.Helper()
	ms, err := database.EmbeddedMigrations("migrations/main")
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	for _, m := range ms {
		if m.Version <= v {
			out[m.Name] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	return out
}
