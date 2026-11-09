package metadata

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// LookupCache keeps provider answers in the lookup_cache table with an
// expiry. It is not the person's data: rows may go at any
// time. It satisfies metadata.Cache.
type LookupCache struct{ Store *database.Store }

// Get returns the body cached under key when it has not expired at now.
func (c LookupCache) Get(ctx context.Context, key string, now time.Time) ([]byte, bool, error) {
	var body []byte
	err := c.Store.Read().QueryRowContext(ctx, "SELECT body FROM lookup_cache WHERE key = ? AND expires_at > ?", key, database.FormatTime(now)).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return body, err == nil, err
}

// Put stores body under key until expires, and drops a few expired rows so
// the table does not grow without end.
func (c LookupCache) Put(ctx context.Context, key string, body []byte, now, expires time.Time) error {
	return c.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO lookup_cache (key, body, fetched_at, expires_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at, expires_at = excluded.expires_at`,
			key, body, database.FormatTime(now), database.FormatTime(expires)); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM lookup_cache WHERE key IN (SELECT key FROM lookup_cache WHERE expires_at <= ? LIMIT 100)", database.FormatTime(now))
		return err
	})
}

// TrimPrefix keeps the newest keep rows whose key starts with prefix and
// drops the older ones.
func (c LookupCache) TrimPrefix(ctx context.Context, prefix string, keep int) error {
	return c.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM lookup_cache WHERE key IN (SELECT key FROM lookup_cache WHERE key LIKE ?
			ORDER BY fetched_at DESC LIMIT -1 OFFSET ?)`, prefix+"%", keep)
		return err
	})
}
