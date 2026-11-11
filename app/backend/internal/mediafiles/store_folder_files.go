package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// FolderFile is one present file in a Library folder: its id, its path
// under the folder and the size the Library recorded for it.
type FolderFile struct {
	ID           int64
	RelativePath string
	Size         int64
}

// PresentFolderFiles lists the present files of every Version in a Library
// folder, in id order.
func PresentFolderFiles(ctx context.Context, db database.Lister, folderID int64) ([]FolderFile, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.relative_path, f.size FROM files f JOIN versions v ON v.id = f.version_id
		WHERE v.library_folder_id = ? AND f.state = 'present' ORDER BY f.id`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FolderFile
	for rows.Next() {
		var f FolderFile
		if err := rows.Scan(&f.ID, &f.RelativePath, &f.Size); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
