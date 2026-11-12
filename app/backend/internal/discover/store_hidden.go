package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// errHideUser: the person a hide is for does not exist.
var errHideUser = errors.New("no such person")

// readHides reads every hide, oldest first.
func readHides(ctx context.Context, db database.Lister) ([]Hide, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, created_at, kind, tmdb_id, genre, name, ifnull(user_id, 0) FROM hidden ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hide{}
	for rows.Next() {
		var x Hide
		if err := rows.Scan(&x.ID, &x.CreatedAt, &x.Kind, &x.TMDBID, &x.Genre, &x.Name, &x.UserID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// insertHide adds x unless the same hide is there already; either way it
// returns the stored row, and created says whether it is new.
func insertHide(ctx context.Context, s *database.Store, x Hide, by database.Actor) (Hide, bool, error) {
	who, err := json.Marshal(by)
	if err != nil {
		return x, false, err
	}
	var user any
	if x.UserID != 0 {
		user = x.UserID
	}
	created := false
	err = s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO hidden (created_at, created_by, kind, tmdb_id, genre, name, user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			time.Now().UTC().Format(time.RFC3339), string(who), x.Kind, x.TMDBID, x.Genre, x.Name, user)
		if err != nil {
			if strings.Contains(err.Error(), "FOREIGN KEY") {
				return errHideUser
			}
			return err
		}
		n, _ := res.RowsAffected()
		created = n > 0
		return tx.QueryRowContext(ctx, `SELECT id, created_at, name FROM hidden
			WHERE kind = ? AND tmdb_id = ? AND genre = ? AND ifnull(user_id, 0) = ?`,
			x.Kind, x.TMDBID, x.Genre, x.UserID).Scan(&x.ID, &x.CreatedAt, &x.Name)
	})
	if err == nil {
		forgetHidden(s)
	}
	return x, created, err
}

// deleteHide removes hide id; false when there was none.
func deleteHide(ctx context.Context, s *database.Store, id int64) (bool, error) {
	var n int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM hidden WHERE id = ?`, id)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	if err == nil && n > 0 {
		forgetHidden(s)
	}
	return n > 0, err
}
