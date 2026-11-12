package accounts

import (
	"context"
	"encoding/json"
)

// savedViews reads every view pick a person saved, by name.
func savedViews(ctx context.Context, q conn, userID int64) (map[string]json.RawMessage, error) {
	rows, err := q.QueryContext(ctx, "SELECT name, value FROM user_views WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		out[name] = json.RawMessage(value)
	}
	return out, rows.Err()
}

// putSavedView saves one view pick, replacing the one saved before.
func putSavedView(ctx context.Context, q conn, userID int64, name, value string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO user_views (user_id, name, value) VALUES (?, ?, ?)
			ON CONFLICT (user_id, name) DO UPDATE SET value = excluded.value`, userID, name, value)
	return err
}

// forgetSavedView removes one view pick, so the page opens on its default.
func forgetSavedView(ctx context.Context, q conn, userID int64, name string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM user_views WHERE user_id = ? AND name = ?", userID, name)
	return err
}
