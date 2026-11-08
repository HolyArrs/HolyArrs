package health

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// libraryFolders reads every Library folder's path, kind and label, by kind
// and path. The folders belong to mediafiles; this read moves to its service
// when that module has one.
func libraryFolders(ctx context.Context, q database.Lister) ([]DiskView, error) {
	rows, err := q.QueryContext(ctx, "SELECT path, kind, label FROM library_folders ORDER BY kind, path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiskView
	for rows.Next() {
		var d DiskView
		if err := rows.Scan(&d.Path, &d.Kind, &d.Label); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// sqliteVersion is the SQLite library's version; "" when it cannot be read.
func sqliteVersion(ctx context.Context, q database.Querier) string {
	var v string
	_ = q.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&v)
	return v
}
