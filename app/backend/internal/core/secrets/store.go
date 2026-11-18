package secrets

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// FirstSaved is when the first secret worth protecting was saved; zero when
// there is none. Secrets HolyArrs keeps for itself do not count: the key
// check value, and settings it saves on its own (its sign-up with the
// metadata server), which every install gets on first start.
func FirstSaved(ctx context.Context, s *database.Store) (time.Time, error) {
	var at sql.NullString
	err := s.Read().QueryRowContext(ctx, "SELECT MIN(created_at) FROM secrets WHERE kind != ? AND NOT (created_by = ? AND kind LIKE 'setting:%')",
		checkKind, actor(database.System)).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !at.Valid {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(database.TimeLayout, at.String)
}
