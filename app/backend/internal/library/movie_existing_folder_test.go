package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestAddMovieUsesExistingFolder: a movie added while its files sit in
// a folder whose name differs only by case, accents or dots keeps that
// folder, like shows do, so no second folder is made beside it.
func TestAddMovieUsesExistingFolder(t *testing.T) {
	m := metadata.Movie{Name: "Élément perturbateur", Year: 2023, IDs: map[string]string{"tmdb": "437001"}, CinemaDate: d("2023-03-01")}
	for _, dir := range []string{"Élément Perturbateur (2023)", "Element.Perturbateur.2023", "élément perturbateur (2023) [tmdbid-437001]"} {
		t.Run(dir, func(t *testing.T) {
			s := open(t)
			ctx := context.Background()
			lf := addFolder(t, s)
			for _, x := range []string{dir, "Élément Perturbateur 2 (2023)"} {
				if err := os.MkdirAll(filepath.Join(lf.Path, x), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			id, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			var got string
			if err := s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != dir && !(FoldsCase(lf.Path) && strings.EqualFold(got, dir)) {
				t.Fatalf("folder %q, want %q", got, dir)
			}
		})
	}
}

// TestPlanAdoptCaseOnlyFolderOnCaseSensitiveDisk: on a disk that tells
// case apart, a second folder that differs from a Library movie's folder only
// by case is its own folder, so the scan does not call it already in the Library.
func TestPlanAdoptCaseOnlyFolderOnCaseSensitiveDisk(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	if FoldsCase(lf.Path) {
		t.Skip("disk folds case")
	}
	m := metadata.Movie{Name: "Élément Perturbateur", Year: 2023, IDs: map[string]string{"tmdb": "437001"}, CinemaDate: d("2023-03-01")}
	if _, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, x := range []string{"Élément Perturbateur (2023)", "Élément perturbateur (2023)"} {
		if err := os.MkdirAll(filepath.Join(lf.Path, x), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := PlanAdopt(ctx, s.Read(), adoptTMDB, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Folders {
		if (c.Status == "in_library") != (c.Folder == "Élément Perturbateur (2023)") {
			t.Fatalf("%q status %q", c.Folder, c.Status)
		}
	}
}
