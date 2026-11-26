package mediafiles

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// standInPath is the path of the stand-in folder of a kind. It is not a
// path on disk and is never opened.
func standInPath(kind string) string { return "hands-off:" + kind }

// StandInFolder returns the stand-in Library folder of kind (movie, show or
// music), making it the first time. See LibraryFolder.StandIn.
func StandInFolder(ctx context.Context, s *database.Store, kind string, by domain.Actor) (LibraryFolder, error) {
	var lf LibraryFolder
	find := func(q database.Lister) error {
		fs, err := database.Select[LibraryFolder](ctx, q, "library_folders", "stand_in = 1 AND kind = ? LIMIT 1", kind)
		if err != nil {
			return err
		}
		if len(fs) == 0 {
			return database.ErrNotFound
		}
		lf = fs[0]
		return nil
	}
	if err := find(s.Read()); !errors.Is(err, database.ErrNotFound) {
		return lf, err
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := find(tx); !errors.Is(err, database.ErrNotFound) {
			return err
		}
		lf = LibraryFolder{Path: standInPath(kind), Kind: kind, Label: kind, StandIn: true, Health: "ok", SameDiskAs: []int64{}}
		id, err := database.Insert(tx, "library_folders", &lf, by)
		lf.ID = id
		return err
	})
	return lf, err
}

// FirstRealFolder returns the first real Library folder of kind, the one a
// new title goes into when none is named (false when there is none).
func FirstRealFolder(ctx context.Context, q database.Lister, kind string) (LibraryFolder, bool, error) {
	fs, err := database.Select[LibraryFolder](ctx, q, "library_folders", "kind = ? AND stand_in = 0 ORDER BY look_only, id LIMIT 1", kind)
	if err != nil || len(fs) == 0 {
		return LibraryFolder{}, false, err
	}
	return fs[0], true, nil
}

// SlotStandIn says whether the Slot id belongs to a copy kept under a
// stand-in folder.
func SlotStandIn(ctx context.Context, q database.Querier, slotID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, `SELECT count(*) FROM slots s JOIN versions v ON v.id = s.version_id
		JOIN library_folders lf ON lf.id = v.library_folder_id WHERE s.id = ? AND lf.stand_in = 1`, slotID).Scan(&n)
	return n > 0
}
