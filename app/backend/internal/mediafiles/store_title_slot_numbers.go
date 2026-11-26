package mediafiles

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// NumberedSlot is a Slot with its item's season and episode (both 0 for a
// movie), so a release name can be matched to it.
type NumberedSlot struct {
	ID       int64
	Season   int
	Episode  int
	Absolute int    // anime running number, 0 when unknown
	AirDate  string // YYYY-MM-DD, empty when unknown
}

// TitleSlotNumbers lists every Slot of a title that is not removed.
func TitleSlotNumbers(ctx context.Context, q database.Lister, titleID int64) ([]NumberedSlot, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id, COALESCE(json_extract(i.numbers, '$.season'), 0), COALESCE(json_extract(i.numbers, '$.episode'), 0),
		COALESCE(json_extract(i.numbers, '$.absolute'), 0), COALESCE(substr(i.air_date, 1, 10), '')
		FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND s.state != 'removed' ORDER BY s.id`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NumberedSlot
	for rows.Next() {
		var n NumberedSlot
		var se, ep, ab sql.NullInt64
		if err := rows.Scan(&n.ID, &se, &ep, &ab, &n.AirDate); err != nil {
			return nil, err
		}
		n.Season, n.Episode, n.Absolute = int(se.Int64), int(ep.Int64), int(ab.Int64)
		out = append(out, n)
	}
	return out, rows.Err()
}
