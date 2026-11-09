package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A show or an artist moves to another Library folder of its own kind,
// sub-folders (seasons, albums) and all; a folder of another kind is refused.
func TestMoveShowAndArtistToTheirOwnFolderKind(t *testing.T) {
	for _, c := range []struct {
		kind, folder, rel string
		wrong             error
	}{
		{"show", "show", "Dune - Part Two (2024)/Season 01/Dune - S01E01.mkv", ErrNotShowFolder},
		{"artist", "music", "Dune - Part Two (2024)/Harbour Songs (2019)/01 - Song 1.1.flac", ErrNotMusicFolder},
	} {
		t.Run(c.kind, func(t *testing.T) {
			_, s, from, id := renameWorld(t)
			ctx := context.Background()
			if err := s.Write(ctx, func(tx *sql.Tx) error {
				if _, err := tx.Exec("UPDATE titles SET kind = ? WHERE id = ?", c.kind, id); err != nil {
					return err
				}
				_, err := tx.Exec("UPDATE library_folders SET kind = ? WHERE id = ?", c.folder, from.ID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			putFile(t, s, from, id, c.rel, "x", "adopted")
			movies, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie"}, nil, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PlanMove(ctx, s.Read(), s.Read(), id, movies.ID); !errors.Is(err, c.wrong) {
				t.Fatalf("a movie folder for a %s: %v, want %v", c.kind, err, c.wrong)
			}
			to, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: c.folder}, nil, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			svc, _, _ := runQueue(t, s, nil)
			if _, err := svc.Move(ctx, id, to.ID, true, domain.System); err != nil {
				t.Fatal(err)
			}
			if js := waitJobs(t, s, 1); js[0].State != "done" {
				t.Fatalf("move job %+v", js[0])
			}
			if _, err := os.Stat(filepath.Join(to.Path, filepath.FromSlash(c.rel))); err != nil {
				t.Fatalf("file not in the new folder: %v", err)
			}
			// The emptied season or album folder goes with the old title folder.
			if _, err := os.Stat(filepath.Join(from.Path, "Dune - Part Two (2024)")); !os.IsNotExist(err) {
				t.Fatalf("old folder left behind: %v", err)
			}
			vs, err := database.Select[mediafiles.Version](ctx, s.Read(), "versions", "title_id = ? AND is_main = 1", id)
			if err != nil || len(vs) != 1 || vs[0].LibraryFolderID != to.ID {
				t.Fatalf("version %+v %v, want folder %d", vs, err, to.ID)
			}
		})
	}
}
