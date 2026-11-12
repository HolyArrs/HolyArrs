package subtitles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// stopgapRows lists the subtitles beside a File that a machine made: HolyArrs's
// own translations and machine-made site files.
func stopgapRows(ctx context.Context, db *sql.DB, fileID int64) ([]Subtitle, error) {
	return database.Select[Subtitle](ctx, db, "subtitles", "file_id = ? AND place = 'beside' AND state = 'present' AND made IS NOT NULL", fileID)
}

// translationHold is how long a want whose translation failed waits before
// the timed search tries to translate it again, so a broken key or model
// does not spend on every pass.
const translationHold = 24 * time.Hour

// holdTranslation moves a want's next try at least a day on after its
// translation failed. The hold lives on the wish row, so it lasts a restart.
func (s *Service) holdTranslation(ctx context.Context, fileID int64, w Want) {
	until := database.FormatTime(now().Add(translationHold))
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE subtitles SET next_try_at = max(COALESCE(next_try_at, ''), ?), updated_at = ?
			WHERE file_id = ? AND place = 'wish' AND language = ? AND forced = ?`, until, database.FormatTime(now()), fileID, w.Language, w.Forced)
		return err
	})
}

// touchStopgap records that a search for something better than a
// machine-made subtitle ran, so the weekly look moves on to the next one.
func (s *Service) touchStopgap(ctx context.Context, id int64) {
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE subtitles SET last_tried_at = ?, updated_at = ? WHERE id = ?", database.FormatTime(now()), database.FormatTime(now()), id)
		return err
	})
}

// stopgapsDue lists Files whose machine-made subtitle was last looked past
// for a week or more, oldest first, for the better subtitles schedule.
func stopgapsDue(ctx context.Context, db *sql.DB, limit int) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT file_id FROM subtitles WHERE place = 'beside' AND state = 'present' AND origin = 'fetched' AND made IS NOT NULL
		GROUP BY file_id HAVING min(COALESCE(last_tried_at, saved_at, created_at)) <= ? ORDER BY min(COALESCE(last_tried_at, saved_at, created_at)) LIMIT ?`,
		database.FormatTime(now().Add(-stopgapRecheck)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// stopgapRecheck is how often the better subtitles schedule looks again for a
// file made by a person to replace a machine-made one.
const stopgapRecheck = 7 * 24 * time.Hour
