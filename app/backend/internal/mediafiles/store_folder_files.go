package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// FolderFile is one present file in a Library folder: its id, its path
// under the folder, the size recorded at import and the size the last
// rescan read (0 when none did; a file re-encoded or retagged in place).
type FolderFile struct {
	ID           int64
	RelativePath string
	Size, Seen   int64
}

// PresentFolderFiles lists the present files of every Version in a Library
// folder, in id order. Titles deleted from the Library are left out, as a
// rescan leaves them out.
func PresentFolderFiles(ctx context.Context, db database.Lister, folderID int64) ([]FolderFile, error) {
	rows, err := db.QueryContext(ctx, `SELECT f.id, f.relative_path, f.size, coalesce(CAST(json_extract(f.media_info, '$.inspection_size') AS INTEGER), 0)
		FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id
		WHERE v.library_folder_id = ? AND t.state != 'removed' AND f.state = 'present' ORDER BY f.id`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FolderFile
	for rows.Next() {
		var f FolderFile
		if err := rows.Scan(&f.ID, &f.RelativePath, &f.Size, &f.Seen); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
