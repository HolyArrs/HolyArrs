package mediaservers

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// tracksDone is the files already set on one server, with the lists they
// were set for.
func tracksDone(ctx context.Context, q database.Lister, serverID int64) (map[string]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT part_key, choice FROM media_tracks WHERE media_server_id = ?", serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, c string
		if err := rows.Scan(&k, &c); err != nil {
			return nil, err
		}
		out[k] = c
	}
	return out, rows.Err()
}

func markTracks(ctx context.Context, s *database.Store, serverID int64, part, choice string) error {
	return s.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO media_tracks(media_server_id, part_key, choice, set_at) VALUES (?, ?, ?, ?) ON CONFLICT(media_server_id, part_key) DO UPDATE SET choice = excluded.choice, set_at = excluded.set_at",
			serverID, part, choice, database.FormatTime(time.Now().UTC()))
		return err
	})
}
