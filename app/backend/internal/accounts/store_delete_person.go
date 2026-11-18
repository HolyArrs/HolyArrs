package accounts

import (
	"context"
	"strconv"
)

// removedName is what a deleted person's row keeps after their name: their real name stays, so
// their asks and history still say who it was, with an invisible separator and
// their id after it. New names refuse that separator, so the stored name never
// matches one anybody types, the real name is free again, and the page can tell
// the person is deleted.
func removedName(id int64) string { return "\u2063" + strconv.FormatInt(id, 10) }

// personToDelete reads the role of a person who has not been deleted yet.
func personToDelete(ctx context.Context, q conn, id int64) (string, error) {
	var role string
	err := q.QueryRowContext(ctx, "SELECT role FROM users WHERE id = ? AND removed_at = ''", id).Scan(&role)
	return role, err
}

// removePersonRow marks a person's name deleted, takes away every way in and the things
// that were only theirs, and marks the row deleted. The row stays so asks,
// downloads and history that point at it keep working.
func removePersonRow(ctx context.Context, q conn, id int64, now, by string) error {
	for _, stmt := range []string{
		"DELETE FROM backup_codes WHERE user_id = ?",
		"DELETE FROM user_profiles WHERE user_id = ?",
		"DELETE FROM user_avatars WHERE user_id = ?",
		"DELETE FROM plex_watchlist_people WHERE user_id = ?",
		"DELETE FROM invites WHERE created_by = ? AND used_at IS NULL",
		"DELETE FROM compat_keys WHERE owner_user_id = ?",
		"DELETE FROM user_ask_settings WHERE user_id = ?",
		"UPDATE ask_people SET follow = 0 WHERE user_id = ?",
		"DELETE FROM hidden WHERE user_id = ?",
		"DELETE FROM user_views WHERE user_id = ?",
		"DELETE FROM follows WHERE user_id = ?",
		"DELETE FROM tmdb_follows WHERE user_id = ?",
		"DELETE FROM tmdb_movie_follows WHERE user_id = ?",
		"DELETE FROM album_follows WHERE user_id = ?",
		"DELETE FROM plex_watchlist_items WHERE user_id = ?",
		"DELETE FROM explore_pool WHERE member_id = ?",
		"DELETE FROM explore_prefs WHERE member_id = ?",
		"DELETE FROM explore_counts WHERE member_id = ?",
		"DELETE FROM explore_titles WHERE member_id = ?",
		"DELETE FROM explore_title_counts WHERE member_id = ?",
		"DELETE FROM explore_category_counts WHERE member_id = ?",
	} {
		if _, err := q.ExecContext(ctx, stmt, id); err != nil {
			return err
		}
	}
	_, err := q.ExecContext(ctx, `UPDATE users SET name = name || ?, password_hash = '', state = 'disabled', perms = '[]',
		removed_at = ?, updated_at = ?, updated_by = ? WHERE id = ?`, removedName(id), now, now, by, id)
	return err
}
