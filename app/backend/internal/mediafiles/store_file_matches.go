package mediafiles

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// FileMatch identifies the catalog records for one present main file.
type FileMatch struct {
	ID, TitleID, VersionID int64
}

// MatchPresentMainFile finds a unique file whose full path matches. The caller
// supplies the comparison because a connected server may see a different path.
func MatchPresentMainFile(ctx context.Context, q database.Lister, titleID *int64, matchesPath func(string) bool) (FileMatch, error) {
	rows, err := q.QueryContext(ctx, `SELECT f.id, v.title_id, v.id, lf.path, f.relative_path
		FROM versions v JOIN files f ON f.version_id = v.id
		JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE f.state = 'present' AND f.role = 'main' AND (? IS NULL OR v.title_id = ?)`, titleID, titleID)
	if err != nil {
		return FileMatch{}, err
	}
	defer rows.Close()
	var match FileMatch
	for rows.Next() {
		var candidate FileMatch
		var root, rel string
		if err := rows.Scan(&candidate.ID, &candidate.TitleID, &candidate.VersionID, &root, &rel); err != nil {
			return FileMatch{}, err
		}
		if !matchesPath(filepath.Join(root, filepath.FromSlash(rel))) {
			continue
		}
		if match.ID != 0 {
			return FileMatch{}, nil
		}
		match = candidate
	}
	return match, rows.Err()
}

type catalogLabel struct {
	ID    int64  `db:"id"`
	Label string `db:"label"`
}

// TitlePosters reads the first catalog poster for each requested title.
func TitlePosters(ctx context.Context, q database.Lister, ids []int64) (map[int64]string, error) {
	return catalogLabels(ctx, q, "(SELECT id, COALESCE(json_extract(artwork, '$[0]'), '') AS label FROM titles)", ids)
}

// VersionLabels reads the display label of each requested file version.
func VersionLabels(ctx context.Context, q database.Lister, ids []int64) (map[int64]string, error) {
	return catalogLabels(ctx, q, "versions", ids)
}

func catalogLabels(ctx context.Context, q database.Lister, table string, ids []int64) (map[int64]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	wanted, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := database.Select[catalogLabel](ctx, q, table, "id IN (SELECT value FROM json_each(?))", string(wanted))
	if err != nil {
		return nil, err
	}
	labels := make(map[int64]string, len(rows))
	for _, row := range rows {
		labels[row.ID] = row.Label
	}
	return labels, nil
}
