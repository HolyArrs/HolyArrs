package accounts

import "context"

// displayName is the display name a person set, "" when none.
func displayName(ctx context.Context, q conn, userID int64) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, "SELECT display_name FROM user_profiles WHERE user_id = ?", userID).Scan(&name)
	return name, err
}

// putDisplayName sets a person's display name, or replaces it.
func putDisplayName(ctx context.Context, q conn, userID int64, name string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO user_profiles (user_id, display_name) VALUES (?, ?)
			ON CONFLICT (user_id) DO UPDATE SET display_name = excluded.display_name`, userID, name)
	return err
}
