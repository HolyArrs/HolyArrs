package mediafiles

import (
	"context"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleFolders returns the folders of a title, one per version, main version
// first. A version without a folder name is left out.
func TitleFolders(ctx context.Context, db database.Lister, titleID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT lf.path, v.folder_name FROM versions v JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE v.title_id = ? ORDER BY v.is_main DESC, v.id`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var root, folder string
		if rows.Scan(&root, &folder) == nil && folder != "" {
			out = append(out, filepath.Join(root, folder))
		}
	}
	return out, rows.Err()
}

// TitleKind returns the kind of a title ("" when there is none).
func TitleKind(ctx context.Context, db database.Querier, titleID int64) string {
	var kind string
	_ = db.QueryRowContext(ctx, "SELECT kind FROM titles WHERE id = ?", titleID).Scan(&kind)
	return kind
}

// MainFolder returns a title's kind, the root of its main version's library
// folder and that version's folder name. It fails when the title has no main
// version.
func MainFolder(ctx context.Context, db database.Querier, titleID int64) (kind, root, folder string, err error) {
	err = db.QueryRowContext(ctx, `SELECT t.kind, lf.path, v.folder_name FROM titles t JOIN versions v ON v.title_id = t.id AND v.is_main = 1
		JOIN library_folders lf ON lf.id = v.library_folder_id WHERE t.id = ?`, titleID).Scan(&kind, &root, &folder)
	return kind, root, folder, err
}

// SlotPath returns the library-relative path of the file a slot holds.
func SlotPath(ctx context.Context, db database.Querier, slotID int64) (string, error) {
	var rel string
	err := db.QueryRowContext(ctx, "SELECT f.relative_path FROM slots s JOIN files f ON f.id = s.file_id WHERE s.id = ?", slotID).Scan(&rel)
	return rel, err
}
