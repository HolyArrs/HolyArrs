package accounts

import (
	"context"
	"database/sql"
)

// deleteKnownPlace forgets one place of a sign-in key, or every place of it
// when ipKey is empty.
func deleteKnownPlace(ctx context.Context, q conn, userKey, ipKey string) error {
	query, args := "DELETE FROM known_places WHERE user_key = ? AND ip_key = ?", []any{userKey, ipKey}
	if ipKey == "" {
		query, args = "DELETE FROM known_places WHERE user_key = ?", []any{userKey}
	}
	_, err := q.ExecContext(ctx, query, args...)
	return err
}

// deleteOldKnownPlaces forgets the places of a sign-in key not seen since before.
func deleteOldKnownPlaces(ctx context.Context, q conn, userKey, before string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM known_places WHERE user_key = ? AND last_at < ?", userKey, before)
	return err
}

// putKnownPlace remembers a place for a sign-in key, or renews it.
func putKnownPlace(ctx context.Context, q conn, userKey, ipKey, at string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO known_places(user_key, ip_key, last_at) VALUES (?, ?, ?)
			ON CONFLICT(user_key, ip_key) DO UPDATE SET last_at = excluded.last_at`, userKey, ipKey, at)
	return err
}

// knownPlacesSince lists the places seen since the given time, oldest first.
func knownPlacesSince(ctx context.Context, q conn, since string) (*sql.Rows, error) {
	return q.QueryContext(ctx, "SELECT user_key, ip_key, last_at FROM known_places WHERE last_at >= ? ORDER BY last_at", since)
}
