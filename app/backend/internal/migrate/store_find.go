package migrate

import (
	"context"
	"database/sql"
)

// knownAddresses are the addresses of the latest Snapshots and of the
// Downloaders already added, at most 20 of each.
func knownAddresses(ctx context.Context, db *sql.DB) []string {
	rows, err := db.QueryContext(ctx,
		`SELECT address FROM (SELECT address, id FROM migrate_snapshots ORDER BY id DESC LIMIT 20)
		 UNION ALL SELECT url FROM (SELECT url FROM downloaders ORDER BY id LIMIT 20)`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a sql.NullString
		if rows.Scan(&a) == nil && a.Valid {
			out = append(out, a.String)
		}
	}
	return out
}
