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

// kindGap is a protocol some enabled Source gives and whether a Downloader
// that is not removed takes it.
type kindGap struct {
	Protocol string
	Has      bool
}

// downloaderGaps reads, for each protocol the enabled Sources give, whether
// any Downloader takes it.
func downloaderGaps(ctx context.Context, q database.Lister) ([]kindGap, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.protocol, EXISTS (SELECT 1 FROM downloaders d WHERE d.protocol = s.protocol AND d.removed = 0)
		FROM sources s WHERE s.state = 'enabled' GROUP BY s.protocol ORDER BY s.protocol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kindGap
	for rows.Next() {
		var g kindGap
		if err := rows.Scan(&g.Protocol, &g.Has); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
