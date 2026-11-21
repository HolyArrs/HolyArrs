package youtube

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// InsertFollow adds a follow and returns its id; ErrDuplicateFollow when the
// person already follows the same playlist or channel.
func InsertFollow(tx *sql.Tx, f Follow) (int64, error) {
	res, err := tx.Exec(`INSERT INTO youtube_follows
		(user_id, url, kind, yt_id, title, quality_max_height, include_shorts, keep_last, backfill, enabled, mirror, added_at, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		f.UserID, f.URL, f.Kind, f.YTID, f.Title, maxHeight(f.MaxHeight), f.IncludeShorts, f.KeepLast, f.Backfill, f.Enabled, f.Mirror, database.FormatTime(f.AddedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: youtube_follows.") {
			return 0, ErrDuplicateFollow
		}
		return 0, err
	}
	return res.LastInsertId()
}

// maxHeight falls back to the default cap when none was given.
func maxHeight(h int) int {
	if h <= 0 {
		return 1080
	}
	return h
}

// FollowsOf reads one person's follows, oldest first.
func FollowsOf(ctx context.Context, q database.Lister, userID int64) ([]Follow, error) {
	return database.Select[Follow](ctx, q, "youtube_follows", "user_id = ? ORDER BY id", userID)
}

// FollowOf reads one of a person's follows; sql.ErrNoRows when it is not
// theirs.
func FollowOf(ctx context.Context, q database.Querier, userID, id int64) (Follow, error) {
	var f Follow
	err := q.QueryRowContext(ctx, "SELECT "+database.SelectList(&f)+" FROM youtube_follows WHERE id = ? AND user_id = ?", id, userID).
		Scan(database.ScanInto(&f)...)
	return f, err
}

// UpdateFollow applies a partial edit to one of a person's follows;
// sql.ErrNoRows when it is not theirs.
func UpdateFollow(tx *sql.Tx, userID, id int64, c FollowChange) error {
	var sets []string
	var args []any
	if c.MaxHeight != nil {
		sets, args = append(sets, "quality_max_height = ?"), append(args, maxHeight(*c.MaxHeight))
	}
	if c.IncludeShorts != nil {
		sets, args = append(sets, "include_shorts = ?"), append(args, *c.IncludeShorts)
	}
	if c.Enabled != nil {
		sets, args = append(sets, "enabled = ?"), append(args, *c.Enabled)
	}
	if c.KeepLast != nil {
		sets, args = append(sets, "keep_last = ?"), append(args, *c.KeepLast)
	}
	if c.Mirror != nil {
		sets, args = append(sets, "mirror = ?"), append(args, *c.Mirror)
	}
	if len(sets) == 0 {
		sets = []string{"id = id"}
	}
	args = append(args, id, userID)
	res, err := tx.Exec("UPDATE youtube_follows SET "+strings.Join(sets, ", ")+" WHERE id = ? AND user_id = ?", args...)
	return oneRow(res, err)
}

// MarkChecked keeps when a follow was last checked and why it failed, if it
// did.
func MarkChecked(tx *sql.Tx, id int64, at time.Time, errText string) error {
	_, err := tx.Exec("UPDATE youtube_follows SET last_checked_at = ?, last_error = ? WHERE id = ?", database.FormatTime(at), errText, id)
	return err
}

// MarkCheckFailed keeps why a follow's listing failed, leaving when it was
// last listed as it was.
func MarkCheckFailed(tx *sql.Tx, id int64, errText string) error {
	_, err := tx.Exec("UPDATE youtube_follows SET last_error = ? WHERE id = ?", errText, id)
	return err
}

// DeleteFollow removes one of a person's follows and what it held; the
// videos themselves stay. sql.ErrNoRows when it is not theirs.
func DeleteFollow(tx *sql.Tx, userID, id int64) error {
	if _, err := tx.Exec("DELETE FROM youtube_video_follows WHERE follow_id IN (SELECT id FROM youtube_follows WHERE id = ? AND user_id = ?)", id, userID); err != nil {
		return err
	}
	res, err := tx.Exec("DELETE FROM youtube_follows WHERE id = ? AND user_id = ?", id, userID)
	return oneRow(res, err)
}

// EnabledFollows reads every turned-on follow of every person, for the
// check job only.
func EnabledFollows(ctx context.Context, q database.Lister) ([]Follow, error) {
	return database.Select[Follow](ctx, q, "youtube_follows", "enabled = 1 ORDER BY id")
}

// DeletePersonRows removes every follow and video row of a person.
func DeletePersonRows(tx *sql.Tx, userID int64) error {
	if _, err := tx.Exec("DELETE FROM youtube_video_follows WHERE video_row_id IN (SELECT id FROM youtube_videos WHERE user_id = ?)", userID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM youtube_videos WHERE user_id = ?", userID); err != nil {
		return err
	}
	_, err := tx.Exec("DELETE FROM youtube_follows WHERE user_id = ?", userID)
	return err
}

// oneRow turns a write that touched nothing into sql.ErrNoRows.
func oneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// FollowOfYTID reads a person's follow of one playlist or channel;
// sql.ErrNoRows when they do not follow it.
func FollowOfYTID(ctx context.Context, q database.Querier, userID int64, kind, ytID string) (Follow, error) {
	var f Follow
	err := q.QueryRowContext(ctx, "SELECT "+database.SelectList(&f)+" FROM youtube_follows WHERE user_id = ? AND kind = ? AND yt_id = ?", userID, kind, ytID).
		Scan(database.ScanInto(&f)...)
	return f, err
}
