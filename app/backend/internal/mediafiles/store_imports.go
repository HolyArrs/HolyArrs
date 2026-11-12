package mediafiles

import (
	"context"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// RefusedDownloads reports which of the Downloads had their newest Import
// turned down (the files were refused, whatever the Slot did next).
func RefusedDownloads(ctx context.Context, q database.Lister, ids []int64) (map[int64]bool, error) {
	newest := map[int64]string{}
	for len(ids) > 0 {
		n := min(len(ids), 500)
		part := make([]any, n)
		for i, id := range ids[:n] {
			part[i] = id
		}
		ids = ids[n:]
		rows, err := q.QueryContext(ctx, "SELECT download_id, state FROM imports WHERE download_id IN (?"+strings.Repeat(", ?", n-1)+") ORDER BY id", part...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var state string
			if err := rows.Scan(&id, &state); err != nil {
				rows.Close()
				return nil, err
			}
			newest[id] = state
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	out := map[int64]bool{}
	for id, state := range newest {
		if state == "rejected" {
			out[id] = true
		}
	}
	return out, nil
}
