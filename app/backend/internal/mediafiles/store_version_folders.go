package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// VersionFolderTaken says a Version of a title still in the Library uses
// folder in Library folder lfID, matched without regard to case: the check
// adding a Version makes, so a dry run can say what the real run will do.
func VersionFolderTaken(ctx context.Context, q database.Querier, lfID int64, folder string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM versions WHERE library_folder_id = ? AND lower(folder_name) = lower(?)
		AND title_id IN (SELECT id FROM titles WHERE state != 'removed'))`, lfID, folder).Scan(&ok)
	return ok, err
}
