package accounts

import (
	"context"
	"database/sql"
)

// avatarTag is the etag of a person's picture.
func avatarTag(ctx context.Context, q conn, id int64) (sql.NullString, error) {
	var etag sql.NullString
	err := q.QueryRowContext(ctx, "SELECT etag FROM user_avatars WHERE user_id = ?", id).Scan(&etag)
	return etag, err
}

// putAvatar stores a person's picture, or replaces it.
func putAvatar(ctx context.Context, q conn, userID int64, jpg []byte, etag, at string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO user_avatars(user_id, jpeg, etag, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(user_id) DO UPDATE SET jpeg = excluded.jpeg, etag = excluded.etag, updated_at = excluded.updated_at`,
		userID, jpg, etag, at)
	return err
}

// deleteAvatarOf removes a person's picture.
func deleteAvatarOf(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM user_avatars WHERE user_id = ?", userID)
	return err
}

// avatarOfActive reads the picture of an active person.
func avatarOfActive(ctx context.Context, q conn, id int64) (jpg []byte, etag string, err error) {
	err = q.QueryRowContext(ctx, `SELECT a.jpeg, a.etag FROM user_avatars a JOIN users u ON u.id = a.user_id
		WHERE a.user_id = ? AND u.state = 'active'`, id).Scan(&jpg, &etag)
	return jpg, etag, err
}
