package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func addShowFolder(t *testing.T, s *database.Store) mediafiles.LibraryFolder {
	t.Helper()
	lf, err := AddFolder(context.Background(), s, FolderInput{Path: t.TempDir(), Kind: "show", Label: "Shows"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

// t10Golden compares against testdata/<name>; PIN_UPDATE=1 writes it.
func t10Golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE") == "1" {
		if err := os.WriteFile(p, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden %s missing: %v", p, err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("golden %s differs:\n%s", p, got)
	}
}
