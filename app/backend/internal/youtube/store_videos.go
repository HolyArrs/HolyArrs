package youtube

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// InsertVideo adds a video for a person, held by each of v.FollowIDs. When
// the person already has that video it changes nothing and returns created
// false.
func InsertVideo(tx *sql.Tx, v Video) (id int64, created bool, err error) {
	state := v.State
	if state == "" {
		state = StateQueued
	}
	var uploaded any
	if v.UploadedAt != nil {
		uploaded = database.FormatTime(*v.UploadedAt)
	}
	res, err := tx.Exec(`INSERT INTO youtube_videos
		(user_id, manual, video_id, title, channel_name, channel_id, uploaded_at, duration_seconds, quality_max_height, state, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, video_id) DO NOTHING`,
		v.UserID, v.Manual, v.VideoID, v.Title, v.ChannelName, v.ChannelID, uploaded, v.DurationSeconds, maxHeight(v.MaxHeight), state, database.FormatTime(v.AddedAt))
	if err != nil {
		return 0, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, false, nil
	}
	if id, err = res.LastInsertId(); err != nil {
		return 0, false, err
	}
	for _, f := range v.FollowIDs {
		if err := LinkVideo(tx, v.UserID, id, f, v.AddedAt); err != nil {
			return 0, false, err
		}
	}
	return id, true, nil
}

// VideosOf reads a person's videos, newest added first, leaving out removed
// and skipped ones.
func VideosOf(ctx context.Context, q database.Lister, userID int64, f VideoFilter) ([]Video, error) {
	where := "user_id = ? AND state NOT IN ('removed', 'skipped')"
	args := []any{userID}
	if f.FollowID != 0 {
		where += " AND id IN (SELECT video_row_id FROM youtube_video_follows WHERE follow_id = ? AND let_go = 0)"
		args = append(args, f.FollowID)
	}
	if f.ChannelID != "" {
		where += " AND channel_id = ?"
		args = append(args, f.ChannelID)
	}
	if f.Unwatched {
		where += " AND watched_at IS NULL"
	}
	where += " ORDER BY added_at DESC, id DESC"
	if f.Limit > 0 || f.Offset > 0 {
		limit := f.Limit
		if limit <= 0 {
			limit = -1
		}
		where += " LIMIT ? OFFSET ?"
		args = append(args, limit, max(f.Offset, 0))
	}
	return database.Select[Video](ctx, q, "youtube_videos", where, args...)
}

// VideoOf reads one of a person's videos; sql.ErrNoRows when it is not
// theirs.
func VideoOf(ctx context.Context, q database.Querier, userID, id int64) (Video, error) {
	return videoWhere(ctx, q, "id = ? AND user_id = ?", id, userID)
}

// VideoByID reads any video row, for the jobs only.
func VideoByID(ctx context.Context, q database.Querier, id int64) (Video, error) {
	return videoWhere(ctx, q, "id = ?", id)
}

func videoWhere(ctx context.Context, q database.Querier, where string, args ...any) (Video, error) {
	var v Video
	err := q.QueryRowContext(ctx, "SELECT "+database.SelectList(&v)+" FROM youtube_videos WHERE "+where, args...).
		Scan(database.ScanInto(&v)...)
	return v, err
}

// Known is a person's row for one YouTube video.
type Known struct {
	ID    int64
	State string
}

// KnownVideos is every video a person has a row for, whatever its state,
// by YouTube video id, so a check never adds the same video twice.
func KnownVideos(ctx context.Context, q database.Lister, userID int64) (map[string]Known, error) {
	rows, err := q.QueryContext(ctx, "SELECT video_id, id, state FROM youtube_videos WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Known{}
	for rows.Next() {
		var vid string
		var k Known
		if err := rows.Scan(&vid, &k.ID, &k.State); err != nil {
			return nil, err
		}
		out[vid] = k
	}
	return out, rows.Err()
}

// SetVideoState moves a video to state with its error text.
func SetVideoState(tx *sql.Tx, id int64, state, errText string) error {
	_, err := tx.Exec("UPDATE youtube_videos SET state = ?, error = ? WHERE id = ?", state, errText, id)
	return err
}

// MoveJobVideo moves a video its download job holds to state with its error
// text. sql.ErrNoRows when the row is gone or no longer queued or
// downloading: a removed video stays removed.
func MoveJobVideo(tx *sql.Tx, id int64, state, errText string) error {
	res, err := tx.Exec("UPDATE youtube_videos SET state = ?, error = ? WHERE id = ? AND state IN ('queued', 'downloading')", state, errText, id)
	return oneRow(res, err)
}

// FinishVideo records a finished download: its file, size and what yt-dlp
// told us about it.
func FinishVideo(tx *sql.Tx, id int64, rel string, size int64, m Meta) error {
	var uploaded any
	if m.UploadedAt != nil {
		uploaded = database.FormatTime(*m.UploadedAt)
	}
	_, err := tx.Exec(`UPDATE youtube_videos SET state = 'done', error = '', relative_path = ?, size_bytes = ?,
		title = ?, channel_name = ?, channel_id = ?, uploaded_at = ?, duration_seconds = ? WHERE id = ?`,
		rel, size, m.Title, m.ChannelName, m.ChannelID, uploaded, m.DurationSeconds, id)
	return err
}

// SavePosition keeps where a person stopped in one of their videos. Once a
// video is watched it stays watched, keeping the first time.
// sql.ErrNoRows when the video is not theirs.
func SavePosition(tx *sql.Tx, userID, id int64, seconds int, watched bool, at time.Time) error {
	var res sql.Result
	var err error
	if watched {
		res, err = tx.Exec("UPDATE youtube_videos SET position_seconds = ?, watched_at = COALESCE(watched_at, ?) WHERE id = ? AND user_id = ?",
			max(seconds, 0), database.FormatTime(at), id, userID)
	} else {
		res, err = tx.Exec("UPDATE youtube_videos SET position_seconds = ? WHERE id = ? AND user_id = ?", max(seconds, 0), id, userID)
	}
	return oneRow(res, err)
}

// QueuedVideos reads every queued video of every person, oldest first, for
// the jobs only.
func QueuedVideos(ctx context.Context, q database.Lister) ([]Video, error) {
	return database.Select[Video](ctx, q, "youtube_videos", "state = 'queued' ORDER BY id")
}

// ReadTotals counts downloaded videos, their bytes and the people who have
// any, across everyone. No titles leave this function.
func ReadTotals(ctx context.Context, q database.Querier) (Totals, error) {
	var t Totals
	err := q.QueryRowContext(ctx, "SELECT count(*), COALESCE(sum(size_bytes), 0), count(DISTINCT user_id) FROM youtube_videos WHERE state = 'done'").
		Scan(&t.Videos, &t.Bytes, &t.People)
	return t, err
}

// VideoOfYouTubeID reads a person's row for one YouTube video;
// sql.ErrNoRows when they have none.
func VideoOfYouTubeID(ctx context.Context, q database.Querier, userID int64, videoID string) (Video, error) {
	return videoWhere(ctx, q, "user_id = ? AND video_id = ?", userID, videoID)
}

// RequeueVideo puts one of a person's videos back in the queue at a new
// quality cap, forgetting any old file and error. sql.ErrNoRows when it is
// not theirs.
func RequeueVideo(tx *sql.Tx, userID, id int64, height int) error {
	res, err := tx.Exec("UPDATE youtube_videos SET state = 'queued', error = '', relative_path = '', size_bytes = 0, quality_max_height = ? WHERE id = ? AND user_id = ?",
		maxHeight(height), id, userID)
	return oneRow(res, err)
}

// MarkRemoved records that the person removed one of their videos and its
// file was deleted. The row stays, no longer manual, so a check never
// downloads it again. sql.ErrNoRows when it is not theirs.
func MarkRemoved(tx *sql.Tx, userID, id int64) error {
	res, err := tx.Exec("UPDATE youtube_videos SET state = 'removed', manual = 0, error = '', relative_path = '', size_bytes = 0 WHERE id = ? AND user_id = ?", id, userID)
	return oneRow(res, err)
}
