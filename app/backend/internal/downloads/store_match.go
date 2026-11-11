package downloads

import (
	"context"
	"database/sql"
)

// unsureCategories reads the category each unsure Confirm match row's item
// was last seen in, keyed by downloader and lower-case key. A row whose
// item was never seen by the reader is left out.
func unsureCategories(ctx context.Context, db *sql.DB) (map[itemKey]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT l.downloader_id, l.hash, COALESCE(
		(SELECT i.category FROM dl_items i WHERE i.downloader_id = l.downloader_id AND lower(i.key) = l.hash LIMIT 1),
		(SELECT q.category FROM dl_match_queue q WHERE q.downloader_id = l.downloader_id AND lower(q.key) = l.hash LIMIT 1))
		FROM client_links l WHERE l.state = 'unsure'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[itemKey]string{}
	for rows.Next() {
		var k itemKey
		var cat sql.NullString
		if err := rows.Scan(&k.id, &k.key, &cat); err != nil {
			return nil, err
		}
		if cat.Valid {
			out[k] = cat.String
		}
	}
	return out, rows.Err()
}
