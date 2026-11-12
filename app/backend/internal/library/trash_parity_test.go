package library

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library/trashtest"
)

// TestTrashBuildParity pins what TRaSH sync makes of the test tree, so
// sharing its converter with the old-apps import changes nothing for it.
func TestTrashBuildParity(t *testing.T) {
	c := NewTrashGuides()
	c.Raw, c.API = trashtest.Serve(t, trashtest.Files)
	d, err := c.Load(context.Background(), "sonarr")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Profile("p1")
	got, _ := json.MarshalIndent(d.Build(p), "", " ")
	got = append(got, '\n')
	path := filepath.Join("testdata", "trash_build_parity.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		_ = os.WriteFile(path, got, 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Fatalf("TRaSH build changed:\n%s", got)
	}
}
