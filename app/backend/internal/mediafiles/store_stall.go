package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// StallStreaks reads the stall streak of each named Release that has one.
func StallStreaks(ctx context.Context, db *sql.DB, names []string) (map[string]Stall, error) {
	out := map[string]Stall{}
	if len(names) == 0 {
		return out, nil
	}
	list, _ := json.Marshal(names)
	rows, err := db.QueryContext(ctx, `SELECT name, count, last_at FROM release_stalls WHERE name IN (SELECT value FROM json_each(?))`, string(list))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, at string
		var s Stall
		if err := rows.Scan(&name, &s.Count, &at); err != nil {
			return nil, err
		}
		s.Last, _ = time.Parse(time.RFC3339Nano, at)
		out[name] = s
	}
	return out, rows.Err()
}

// AddStallTx counts one more stall in a row for the named Release and
// returns the count now.
func AddStallTx(tx *sql.Tx, name string, at time.Time) (int, error) {
	var n int
	err := tx.QueryRow(`INSERT INTO release_stalls (name, count, last_at) VALUES (?, 1, ?)
		ON CONFLICT (name) DO UPDATE SET count = count + 1, last_at = excluded.last_at RETURNING count`,
		name, database.FormatTime(at)).Scan(&n)
	return n, err
}

// ClearStall ends the named Release's stall streak: one of its Downloads finished.
func ClearStall(ctx context.Context, s *database.Store, name string) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM release_stalls WHERE name = ?`, name)
		return err
	})
}
