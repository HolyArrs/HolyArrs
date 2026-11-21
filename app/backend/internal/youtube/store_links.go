package youtube

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A video is held while it is manual or a follow's link to it is not let
// go. otherHolds is true for a video row some follow still holds.
const otherHolds = "EXISTS (SELECT 1 FROM youtube_video_follows l WHERE l.video_row_id = youtube_videos.id AND l.let_go = 0)"

// LinkVideo records that one of a person's follows holds one of their
// videos. It changes nothing when the two are already linked, a link let go
// included, or when either is not theirs.
func LinkVideo(tx *sql.Tx, userID, rowID, followID int64, at time.Time) error {
	_, err := tx.Exec(`INSERT OR IGNORE INTO youtube_video_follows (video_row_id, follow_id, added_at)
		SELECT v.id, f.id, ? FROM youtube_videos v, youtube_follows f
		WHERE v.id = ? AND f.id = ? AND v.user_id = ? AND f.user_id = ?`,
		database.FormatTime(at), rowID, followID, userID, userID)
	return err
}

// UnlinkVideo forgets that one of a person's follows had one of their
// videos, so the follow may take it again later.
func UnlinkVideo(tx *sql.Tx, userID, rowID, followID int64) error {
	_, err := tx.Exec(`DELETE FROM youtube_video_follows WHERE video_row_id = ? AND follow_id = ?
		AND video_row_id IN (SELECT id FROM youtube_videos WHERE user_id = ?)`, rowID, followID, userID)
	return err
}

// LetGoVideo records that one of a person's follows let one of their videos
// go: the follow no longer holds it and never takes it back.
func LetGoVideo(tx *sql.Tx, userID, rowID, followID int64) error {
	_, err := tx.Exec(`UPDATE youtube_video_follows SET let_go = 1 WHERE video_row_id = ? AND follow_id = ?
		AND video_row_id IN (SELECT id FROM youtube_videos WHERE user_id = ?)`, rowID, followID, userID)
	return err
}

// TakeBackVideo undoes LetGoVideo: the follow holds the video again. A
// playlist that starts being mirrored takes back what keep-last let go.
func TakeBackVideo(tx *sql.Tx, userID, rowID, followID int64) error {
	_, err := tx.Exec(`UPDATE youtube_video_follows SET let_go = 0 WHERE video_row_id = ? AND follow_id = ?
		AND video_row_id IN (SELECT id FROM youtube_videos WHERE user_id = ?)`, rowID, followID, userID)
	return err
}

// SetManual marks one of a person's videos as added by its own link.
// sql.ErrNoRows when it is not theirs.
func SetManual(tx *sql.Tx, userID, rowID int64) error {
	res, err := tx.Exec("UPDATE youtube_videos SET manual = 1 WHERE id = ? AND user_id = ?", rowID, userID)
	return oneRow(res, err)
}

// KeepVideosOfFollow marks the videos one of a person's follows holds as
// manual, for an unfollow that keeps them: no other follow deletes them
// later.
func KeepVideosOfFollow(tx *sql.Tx, userID, followID int64) error {
	_, err := tx.Exec(`UPDATE youtube_videos SET manual = 1 WHERE user_id = ? AND state NOT IN ('removed', 'skipped')
		AND id IN (SELECT video_row_id FROM youtube_video_follows WHERE follow_id = ? AND let_go = 0)`, userID, followID)
	return err
}

// DeleteVideoIfUnheld deletes one of a person's video rows when it is not
// manual and no follow holds it, and reports whether it did.
func DeleteVideoIfUnheld(tx *sql.Tx, userID, rowID int64) (bool, error) {
	res, err := tx.Exec("DELETE FROM youtube_videos WHERE id = ? AND user_id = ? AND manual = 0 AND NOT "+otherHolds, rowID, userID)
	return touched(res, err)
}

// RemoveVideoIfUnheld marks one of a person's videos removed when it is not
// manual and no follow holds it, and reports whether it did. The row stays
// so a check never downloads it again.
func RemoveVideoIfUnheld(tx *sql.Tx, userID, rowID int64) (bool, error) {
	res, err := tx.Exec(`UPDATE youtube_videos SET state = 'removed', error = '', relative_path = '', size_bytes = 0
		WHERE id = ? AND user_id = ? AND manual = 0 AND NOT `+otherHolds, rowID, userID)
	return touched(res, err)
}

func touched(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// FollowLink is what a follow has to do with a video: Holds until the follow let
// it go, Missing once a complete listing of a mirrored playlist did not
// have it.
type FollowLink struct {
	Holds, Missing bool
}

// FollowLinks is what one of a person's follows has to do with their
// videos, by video row id.
func FollowLinks(ctx context.Context, q database.Lister, userID, followID int64) (map[int64]FollowLink, error) {
	rows, err := q.QueryContext(ctx, `SELECT l.video_row_id, l.let_go, l.missing FROM youtube_video_follows l
		JOIN youtube_videos v ON v.id = l.video_row_id WHERE v.user_id = ? AND l.follow_id = ?`, userID, followID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]FollowLink{}
	for rows.Next() {
		var id int64
		var letGo, missing bool
		if err := rows.Scan(&id, &letGo, &missing); err != nil {
			return nil, err
		}
		out[id] = FollowLink{Holds: !letGo, Missing: missing}
	}
	return out, rows.Err()
}

// MarkMissing records whether the last complete listing of one of a
// person's mirrored playlists lacked one of their videos.
func MarkMissing(tx *sql.Tx, userID, rowID, followID int64, missing bool) error {
	_, err := tx.Exec(`UPDATE youtube_video_follows SET missing = ? WHERE video_row_id = ? AND follow_id = ?
		AND video_row_id IN (SELECT id FROM youtube_videos WHERE user_id = ?)`, missing, rowID, followID, userID)
	return err
}

// IsMissing reports whether one of a person's videos is still marked
// missing from one of their follows.
func IsMissing(ctx context.Context, q database.Querier, userID, rowID, followID int64) (bool, error) {
	var missing bool
	err := q.QueryRowContext(ctx, `SELECT l.missing FROM youtube_video_follows l JOIN youtube_videos v ON v.id = l.video_row_id
		WHERE l.video_row_id = ? AND l.follow_id = ? AND v.user_id = ?`, rowID, followID, userID).Scan(&missing)
	return missing, err
}

// ClearMissing forgets every missing mark of one of a person's follows, for
// when mirroring is turned on or off: marks never carry over.
func ClearMissing(tx *sql.Tx, userID, followID int64) error {
	_, err := tx.Exec(`UPDATE youtube_video_follows SET missing = 0 WHERE missing = 1
		AND follow_id IN (SELECT id FROM youtube_follows WHERE id = ? AND user_id = ?)`, followID, userID)
	return err
}

// WithFollows fills FollowIDs on a person's videos: the follows that hold
// each, lowest id first, and an empty list for a video none holds.
func WithFollows(ctx context.Context, q database.Lister, userID int64, videos []Video) error {
	rows, err := q.QueryContext(ctx, `SELECT l.video_row_id, l.follow_id FROM youtube_video_follows l
		JOIN youtube_videos v ON v.id = l.video_row_id WHERE v.user_id = ? AND l.let_go = 0 ORDER BY l.follow_id`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	by := map[int64][]int64{}
	for rows.Next() {
		var row, follow int64
		if err := rows.Scan(&row, &follow); err != nil {
			return err
		}
		by[row] = append(by[row], follow)
	}
	for i := range videos {
		videos[i].FollowIDs = by[videos[i].ID]
		if videos[i].FollowIDs == nil {
			videos[i].FollowIDs = []int64{}
		}
	}
	return rows.Err()
}

// FollowsHolding reads the follows that hold one of a person's videos, the
// one that took it first at the front.
func FollowsHolding(ctx context.Context, q database.Lister, userID, rowID int64) ([]Follow, error) {
	return database.Select[Follow](ctx, q, "youtube_follows", `user_id = ? AND id IN
		(SELECT follow_id FROM youtube_video_follows WHERE video_row_id = ? AND let_go = 0)
		ORDER BY (SELECT added_at FROM youtube_video_follows WHERE video_row_id = ? AND follow_id = youtube_follows.id), id`,
		userID, rowID, rowID)
}

// KeepLastExcess reads the downloaded videos one of a person's follows
// holds beyond its n newest by upload date, newest of them first. Videos
// with no upload date count as oldest.
func KeepLastExcess(ctx context.Context, q database.Lister, userID, followID int64, n int) ([]Video, error) {
	return database.Select[Video](ctx, q, "youtube_videos",
		`user_id = ? AND state = 'done' AND id IN (SELECT video_row_id FROM youtube_video_follows WHERE follow_id = ? AND let_go = 0)
		ORDER BY uploaded_at IS NULL, uploaded_at DESC, id DESC LIMIT -1 OFFSET ?`, userID, followID, max(n, 0))
}

// VideosOfFollow reads every video row one of a person's follows holds,
// whatever its state.
func VideosOfFollow(ctx context.Context, q database.Lister, userID, followID int64) ([]Video, error) {
	return database.Select[Video](ctx, q, "youtube_videos",
		"user_id = ? AND id IN (SELECT video_row_id FROM youtube_video_follows WHERE follow_id = ? AND let_go = 0) ORDER BY id", userID, followID)
}

// onlyOfFollow is true for a video row linked to the follow given twice
// that is not manual and that no other follow holds.
const onlyOfFollow = `manual = 0 AND id IN (SELECT video_row_id FROM youtube_video_follows WHERE follow_id = ?)
	AND NOT EXISTS (SELECT 1 FROM youtube_video_follows l WHERE l.video_row_id = youtube_videos.id AND l.let_go = 0 AND l.follow_id != ?)`

// OnlyOfFollow reads the video rows that only one of a person's follows
// keeps: linked to it, not manual and held by no other follow. An unfollow
// that deletes files deletes these.
func OnlyOfFollow(ctx context.Context, q database.Lister, userID, followID int64) ([]Video, error) {
	return database.Select[Video](ctx, q, "youtube_videos", "user_id = ? AND "+onlyOfFollow+" ORDER BY id", userID, followID, followID)
}

// DeleteOnlyOfFollow removes the video rows that only one of a person's
// follows keeps.
func DeleteOnlyOfFollow(tx *sql.Tx, userID, followID int64) error {
	_, err := tx.Exec("DELETE FROM youtube_videos WHERE user_id = ? AND "+onlyOfFollow, userID, followID, followID)
	return err
}
