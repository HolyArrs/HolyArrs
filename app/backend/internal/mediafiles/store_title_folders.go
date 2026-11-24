package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleFolders returns the folders of a title, one per version, main version
// first. A version without a folder name is left out.
func TitleFolders(ctx context.Context, db database.Lister, titleID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT lf.path, v.folder_name FROM versions v JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE v.title_id = ? AND lf.stand_in = 0 ORDER BY v.is_main DESC, v.id`, titleID)
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

// FolderSourceTags lists, for each title, every copy (Version) with the tags
// of its Library folder when that folder asks only tagged Sources; a copy in
// any other folder has no tags. titleID 0 reads every title. While no folder
// asks only tagged Sources the list is empty and no copy is read.
func FolderSourceTags(ctx context.Context, db database.Lister, titleID int64) (map[int64]map[int64][]string, error) {
	some, err := db.QueryContext(ctx, "SELECT 1 FROM library_folders WHERE only_tagged_sources = 1 LIMIT 1")
	if err != nil {
		return nil, err
	}
	on := some.Next()
	if err := some.Close(); err != nil || !on {
		return map[int64]map[int64][]string{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT v.title_id, v.id, t.value FROM versions v
		LEFT JOIN library_folders lf ON lf.id = v.library_folder_id AND lf.only_tagged_sources = 1
		LEFT JOIN json_each(lf.new_title_tags) t ON t.type = 'text'
		WHERE ? = 0 OR v.title_id = ?`, titleID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[int64][]string{}
	for rows.Next() {
		var title, version int64
		var tag sql.NullString
		if err := rows.Scan(&title, &version, &tag); err != nil {
			return nil, err
		}
		if out[title] == nil {
			out[title] = map[int64][]string{}
		}
		out[title][version] = out[title][version]
		if tag.Valid {
			out[title][version] = append(out[title][version], tag.String)
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

// TitleInFolder says whether a version of the title lives in folder under
// the library folder root.
func TitleInFolder(ctx context.Context, db database.Querier, titleID int64, root, folder string) bool {
	var id int64
	_ = db.QueryRowContext(ctx, `SELECT v.title_id FROM versions v JOIN library_folders f ON f.id = v.library_folder_id
		WHERE v.title_id = ? AND f.path = ? AND v.folder_name = ?`, titleID, root, folder).Scan(&id)
	return id != 0
}

// FilePath is the full path of a present media file, by its id.
func FilePath(ctx context.Context, db database.Querier, fileID int64) (string, error) {
	var root, rel string
	err := db.QueryRowContext(ctx, `SELECT lf.path, f.relative_path FROM files f JOIN versions v ON v.id = f.version_id
		JOIN library_folders lf ON lf.id = v.library_folder_id WHERE f.id = ? AND f.state = 'present'`, fileID).Scan(&root, &rel)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

// TitleArtwork reads a title's artwork entries; sql.ErrNoRows for no title.
func TitleArtwork(ctx context.Context, db database.Querier, titleID int64) ([]string, error) {
	var raw sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT artwork FROM titles WHERE id = ?", titleID).Scan(&raw); err != nil {
		return nil, err
	}
	var out []string
	_ = json.Unmarshal([]byte(raw.String), &out)
	return out, nil
}

// SetTitleArtwork writes a title's artwork entries (JSON) with who and when.
func SetTitleArtwork(ctx context.Context, tx *sql.Tx, titleID int64, art, now, by string) error {
	_, err := tx.ExecContext(ctx, "UPDATE titles SET artwork = ?, updated_at = ?, updated_by = ? WHERE id = ?", art, now, by, titleID)
	return err
}
