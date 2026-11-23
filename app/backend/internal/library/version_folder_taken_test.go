package library

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// mediafiles.VersionFolderTaken, which the move's dry run uses, matches a
// Version's folder without regard to case, as adding a Version does, and
// only for titles still in the Library.
func TestVersionFolderTaken(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var folder string
	if err := s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&folder); err != nil || folder == "" {
		t.Fatalf("main Version folder %q: %v", folder, err)
	}
	for name, want := range map[string]bool{folder: true, strings.ToUpper(folder): true, folder + " [4K]": false} {
		if got, err := mediafiles.VersionFolderTaken(ctx, s.Read(), lf.ID, name); err != nil || got != want {
			t.Fatalf("%q: taken %v (%v), want %v", name, got, err, want)
		}
	}
	if got, _ := mediafiles.VersionFolderTaken(ctx, s.Read(), lf.ID+1, folder); got {
		t.Fatal("a folder in another Library folder counted as taken")
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := mediafiles.VersionFolderTaken(ctx, s.Read(), lf.ID, folder); got {
		t.Fatal("a removed title still holds its folder")
	}
}
