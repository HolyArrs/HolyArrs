package mediafiles

import (
	"context"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ErrLookOnly says a Library folder is look-only: HolyArrs shows what is
// in it but never downloads into it, renames, moves, tidies, writes or
// deletes anything there.
var ErrLookOnly = errors.New("this Library folder is look-only: HolyArrs shows what is in it but never changes its files")

// LookOnlySQL is a WHERE part that is true when the Version v (a
// versions row alias) lives in a look-only Library folder.
const LookOnlySQL = `EXISTS (SELECT 1 FROM library_folders lof WHERE lof.id = v.library_folder_id AND lof.look_only = 1)`

// FolderLookOnly says whether the Library folder id is look-only. An
// unknown folder is not.
func FolderLookOnly(ctx context.Context, q database.Querier, id int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM library_folders WHERE id = ? AND look_only = 1", id).Scan(&n)
	return n > 0
}

// VersionLookOnly says whether the Version id lives in a look-only Library folder.
func VersionLookOnly(ctx context.Context, q database.Querier, versionID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM versions v WHERE v.id = ? AND "+LookOnlySQL, versionID).Scan(&n)
	return n > 0
}

// SlotLookOnly says whether the Slot id belongs to a copy in a look-only Library folder.
func SlotLookOnly(ctx context.Context, q database.Querier, slotID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM slots s JOIN versions v ON v.id = s.version_id WHERE s.id = ? AND "+LookOnlySQL, slotID).Scan(&n)
	return n > 0
}

// TitleLookOnly says whether any copy of the Title lives in a look-only Library folder.
func TitleLookOnly(ctx context.Context, q database.Querier, titleID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM versions v WHERE v.title_id = ? AND "+LookOnlySQL, titleID).Scan(&n)
	return n > 0
}

// FolderFlat says whether the Library folder id keeps its movies as files
// directly in it, no folder per movie. An unknown folder is not.
func FolderFlat(ctx context.Context, q database.Querier, id int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM library_folders WHERE id = ? AND flat = 1", id).Scan(&n)
	return n > 0
}
