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

// language is the language a person picked, "" when they follow the browser.
func language(ctx context.Context, q conn, userID int64) (string, error) {
	var l string
	err := q.QueryRowContext(ctx, "SELECT language FROM user_profiles WHERE user_id = ?", userID).Scan(&l)
	return l, err
}

// putLanguage sets a person's language and keeps their display name.
func putLanguage(ctx context.Context, q conn, userID int64, l string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO user_profiles (user_id, display_name, language) VALUES (?, '', ?)
			ON CONFLICT (user_id) DO UPDATE SET language = excluded.language`, userID, l)
	return err
}

// OwnerLanguage is the language the active owner picked, "" when none (or no
// owner yet). Alerts use it for the words they send out.
func OwnerLanguage(ctx context.Context, q conn) string {
	id, _, err := ownerIdentity(ctx, q)
	if err != nil {
		return ""
	}
	l, _ := language(ctx, q, id)
	return l
}
