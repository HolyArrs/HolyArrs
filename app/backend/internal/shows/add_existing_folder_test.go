package shows

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestAddShowUsesExistingFolder: a show re-added while its files
// sit in a folder named a little differently from the naming setup keeps
// that folder, so the scan after the add finds the episodes.
func TestAddShowUsesExistingFolder(t *testing.T) {
	cases := map[string]string{
		"dotted":  "Lab.Show.2025",
		"no year": "Lab Show",
		"id tag":  "Lab Show (2025) [tvdbid-200]",
	}
	for name, dir := range cases {
		t.Run(name, func(t *testing.T) {
			svc, s, _, _ := addShowService(t)
			ctx := context.Background()
			lf := addShowFolder(t, s)
			for _, d := range []string{dir, "Lab Show Extra (2025)"} {
				if err := os.MkdirAll(filepath.Join(lf.Path, d, "Season 01"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			var got string
			if err := s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != dir {
				t.Fatalf("folder %q, want %q", got, dir)
			}
		})
	}
}

// TestAddShowNewFolderWhenNoneMatch keeps the naming setup's folder when
// nothing on disk matches.
func TestAddShowNewFolderWhenNoneMatch(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	if err := os.MkdirAll(filepath.Join(lf.Path, "Other Show (2025)"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	_ = s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&got)
	if got == "Other Show (2025)" || got == "" {
		t.Fatalf("folder %q", got)
	}
}
