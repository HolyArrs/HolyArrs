package mediafiles

import (
	"context"
	"database/sql"
	"path"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// AdoptedFileOwner distinguishes an assigned copy from an unassigned note.
type AdoptedFileOwner struct {
	TitleID  int64
	Assigned bool
}

const adoptedOwnersQuery = `SELECT f.relative_path, v.title_id, json_array_length(f.slot_ids) > 0 FROM files f
		JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id
		WHERE v.library_folder_id = ? AND t.state != 'removed' AND f.state IN ('present','missing')`

// AdoptedFileOwners identifies files independently of shared parent folders.
// An empty only lists assigned copies. A targeted lookup also returns notes,
// so adoption cannot silently create a second record for the same file.
func AdoptedFileOwners(ctx context.Context, db database.Lister, libraryFolderID int64, fold bool, only string) (map[string]AdoptedFileOwner, error) {
	query, args := adoptedOwnersQuery, []any{libraryFolderID}
	if only != "" {
		query += " AND lower(f.relative_path) = lower(?)"
		args = append(args, only)
	} else {
		query += " AND json_array_length(f.slot_ids) > 0"
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]AdoptedFileOwner{}
	for rows.Next() {
		var rel string
		var owner AdoptedFileOwner
		if err := rows.Scan(&rel, &owner.TitleID, &owner.Assigned); err != nil {
			return nil, err
		}
		if fold {
			rel = strings.ToLower(rel)
		}
		// Prefer an assigned copy over an old unassigned discovery note.
		if old, ok := out[rel]; !ok || !old.Assigned {
			out[rel] = owner
		}
	}
	return out, rows.Err()
}

// ShareMovieAncestors prevents older folder imports from discovering sibling
// movies once a file inside that folder has been adopted independently.
func ShareMovieAncestors(ctx context.Context, tx *sql.Tx, libraryFolderID int64, folder string, fold bool) error {
	for dir := folder; ; dir = path.Dir(dir) {
		query := `UPDATE versions SET shared_folder = 1 WHERE shared_folder = 0
			AND library_folder_id = ? AND lower(folder_name) = lower(?)`
		args := []any{libraryFolderID, dir}
		if !fold {
			query += " AND folder_name = ?"
			args = append(args, dir)
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		if dir == "." || path.Dir(dir) == dir {
			return nil
		}
	}
}
