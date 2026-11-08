package requests

import (
	"context"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

type askedByRow struct {
	titleID, userID int64
	name            string
	at              time.Time
}

// askedByRows reads each title's askers, earliest first.
func askedByRows(ctx context.Context, s *database.Store, ids []int64) ([]askedByRow, error) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.Read().QueryContext(ctx, `SELECT a.title_id, p.user_id, u.name, min(p.asked_at) FROM asks a
			JOIN ask_people p ON p.ask_id = a.id JOIN users u ON u.id = p.user_id
			WHERE a.title_id IN (?`+strings.Repeat(", ?", len(ids)-1)+`) GROUP BY a.title_id, p.user_id ORDER BY min(p.asked_at), p.user_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []askedByRow
	for rows.Next() {
		var x askedByRow
		var at string
		if err := rows.Scan(&x.titleID, &x.userID, &x.name, &at); err != nil {
			return nil, err
		}
		// Older rows were written in RFC 3339 rather than the store's layout.
		t, err := time.Parse(database.TimeLayout, at)
		if err != nil {
			t, _ = time.Parse(time.RFC3339Nano, at)
		}
		x.at = t
		out = append(out, x)
	}
	return out, rows.Err()
}
