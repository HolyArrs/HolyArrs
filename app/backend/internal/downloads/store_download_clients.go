package downloads

import (
	"context"
	"encoding/json"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ExternalIDs is each of these Downloads' id in its Downloader, empty when it
// has none yet. It returns the rows read so far along with any error.
func ExternalIDs(ctx context.Context, q database.Lister, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return out, err
	}
	rows, err := q.QueryContext(ctx, "SELECT id, COALESCE(external_id, '') FROM downloads WHERE id IN (SELECT value FROM json_each(?))", string(in))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var ext string
		if err := rows.Scan(&id, &ext); err != nil {
			return out, err
		}
		out[id] = ext
	}
	return out, rows.Err()
}
