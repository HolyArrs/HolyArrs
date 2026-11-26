package mediafiles

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// SkipEpisodesTx switches single episodes of one season of a show off.
// The season and its other episodes keep their choice; a number the season
// lacks is fine. ok is false when the show has no such season.
func SkipEpisodesTx(ctx context.Context, tx *sql.Tx, titleID int64, season int, episodes []int, now time.Time, by database.Actor) (bool, error) {
	var partID int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", titleID, season).Scan(&partID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || len(episodes) == 0 {
		return err == nil, err
	}
	args := []any{database.FormatTime(now), actorJSON(by), partID}
	for _, n := range episodes {
		args = append(args, n)
	}
	_, err = tx.ExecContext(ctx, "UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'episode' AND json_extract(numbers, '$.episode') IN ("+
		strings.TrimSuffix(strings.Repeat("?,", len(episodes)), ",")+")", args...)
	return true, err
}
