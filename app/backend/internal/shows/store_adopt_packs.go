package shows

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"strings"
)

func tvFolderAvailable(ctx context.Context, tx *sql.Tx, libraryID int64, folder string, fold bool) error {
	rows, err := tx.QueryContext(ctx, "SELECT folder_name FROM versions WHERE library_folder_id = ?", libraryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	key := library.FolderKey(folder, fold)
	for rows.Next() {
		var existing string
		if err := rows.Scan(&existing); err != nil {
			return err
		}
		other := library.FolderKey(existing, fold)
		if key == other || strings.HasPrefix(key, other+"/") || strings.HasPrefix(other, key+"/") {
			return fmt.Errorf("this folder overlaps a show already in the Library")
		}
	}
	return rows.Err()
}
