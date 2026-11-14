package accounts

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// sessionRow is the stored side of one session.
type sessionRow struct {
	perms, created, rotated, seen, expires, localAuthAt, family string
}

// sessionFamily is the public id of the session a cookie hash belongs to.
func sessionFamily(ctx context.Context, q conn, hash string) (string, error) {
	var family string
	err := q.QueryRowContext(ctx, "SELECT family FROM sessions WHERE hash = ?", hash).Scan(&family)
	return family, err
}

// sessionBy reads the session behind a cookie hash and its active person
// into who.
func sessionBy(ctx context.Context, q conn, hash string, who *webserver.Caller) (sessionRow, error) {
	var s sessionRow
	err := q.QueryRowContext(ctx, `SELECT `+userCols+`, s.csrf, s.created_at, s.rotated_at, s.last_seen_at, s.expires_at, s.auth_method, s.local_auth_at, s.family, COALESCE(u.password_hash, '') = ''
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.hash = ? AND u.state = 'active'`, hash).
		Scan(&who.UserID, &who.Name, &who.Role, &s.perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry, &who.CSRF, &s.created, &s.rotated, &s.seen, &s.expires, &who.AuthMethod, &s.localAuthAt, &s.family, &who.NoPassword)
	return s, err
}

// sessionStill reads a session again for a stream that is already open: the
// newest session of family when one is given, else the one with this hash.
func sessionStill(ctx context.Context, q conn, hash, family string, fresh *webserver.Caller) (perms, seen, expires string, err error) {
	where, arg := `WHERE s.hash = ?`, any(hash)
	if family != "" {
		where, arg = `WHERE s.family = ? ORDER BY s.replaced, s.rotated_at DESC LIMIT 1`, family
	}
	err = q.QueryRowContext(ctx, `SELECT `+userCols+`, s.last_seen_at, s.expires_at
			FROM sessions s JOIN users u ON u.id = s.user_id AND u.state = 'active' `+where, arg).
		Scan(&fresh.UserID, &fresh.Name, &fresh.Role, &perms, &fresh.TwoFactor, &fresh.MaxRating, &fresh.RatingCountry, &seen, &expires)
	return perms, seen, expires, err
}

// insertSession stores a new session.
func insertSession(ctx context.Context, q conn, hash string, userID int64, csrf, now, expires, userAgent, family, method, localAt string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family, auth_method, local_auth_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, hash, userID, csrf, now, now, now, expires, userAgent, family, method, localAt)
	return err
}

// copySessionRotated stores the next cookie of a session: a copy under the
// new hash that carries the rotation time.
func copySessionRotated(ctx context.Context, q conn, newHash, now, oldHash string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sessions(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family, auth_method, local_auth_at)
				SELECT ?, user_id, csrf, created_at, ?, ?, expires_at, user_agent, family, auth_method, local_auth_at FROM sessions WHERE hash = ?`, newHash, now, now, oldHash)
	return err
}

// retireSession lets a replaced cookie live on until expires.
func retireSession(ctx context.Context, q conn, expires, rotated, hash string) error {
	_, err := q.ExecContext(ctx, "UPDATE sessions SET expires_at = ?, rotated_at = ?, replaced = 1 WHERE hash = ?", expires, rotated, hash)
	return err
}

// touchSession records that a session was used.
func touchSession(ctx context.Context, q conn, seen, hash string) error {
	_, err := q.ExecContext(ctx, "UPDATE sessions SET last_seen_at = ? WHERE hash = ?", seen, hash)
	return err
}

// deleteSession ends the session behind a cookie hash.
func deleteSession(ctx context.Context, q conn, hash string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE hash = ?", hash)
	return err
}

// deleteSessionFamily ends a session and all its rotations.
func deleteSessionFamily(ctx context.Context, q conn, family string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE family = ?", family)
	return err
}

// markSetupDone drops the setup prefix from the family of the session behind
// hash, so it becomes an ordinary session.
func markSetupDone(ctx context.Context, q conn, prefixLen int, hash string) error {
	_, err := q.ExecContext(ctx, `UPDATE sessions SET family = substr(family, ?) WHERE family LIKE 'setup.%' AND family = (SELECT family FROM sessions WHERE hash = ?)`,
		prefixLen, hash)
	return err
}

// endedSessionsExist says whether any session is past its end or idle limit.
func endedSessionsExist(ctx context.Context, q conn, expiresAt, idleBefore string) (bool, error) {
	var needed bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM sessions WHERE expires_at <= ? OR last_seen_at < ?
	)`, expiresAt, idleBefore).Scan(&needed)
	return needed, err
}

// deleteEndedSessions drops every session past its end or idle limit.
func deleteEndedSessions(ctx context.Context, q conn, expiresAt, idleBefore string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ? OR last_seen_at < ?",
		expiresAt, idleBefore)
	return err
}

// liveSessions lists a person's live sessions, newest use first.
func liveSessions(ctx context.Context, q conn, userID int64, now time.Time, idle time.Duration, current string) ([]SessionView, error) {
	rows, err := q.QueryContext(ctx, `SELECT family, created_at, last_seen_at, expires_at, user_agent FROM sessions
		WHERE user_id = ? AND replaced = 0 AND expires_at > ? AND last_seen_at >= ? ORDER BY last_seen_at DESC, created_at DESC`,
		userID, database.FormatTime(now), database.FormatTime(now.Add(-idle)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionView{}
	for rows.Next() {
		var fam, created, seen, expires, ua string
		if err := rows.Scan(&fam, &created, &seen, &expires, &ua); err != nil {
			return nil, err
		}
		out = append(out, SessionView{ID: fam, Device: Device(ua), Created: parseTime(created), LastUsed: parseTime(seen),
			Expires: parseTime(expires), Current: current != "" && fam == current})
	}
	return out, rows.Err()
}

// deleteUserSession ends one of a person's sessions by its public id.
func deleteUserSession(ctx context.Context, q conn, userID int64, family string) (sql.Result, error) {
	return q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND family = ?", userID, family)
}

// deleteUserSessionsExcept ends every session of a person but one family.
func deleteUserSessionsExcept(ctx context.Context, q conn, userID int64, keep string) (sql.Result, error) {
	return q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND family != ?", userID, keep)
}

// deleteSessionsByMethod ends a person's sessions that began with one
// sign-in method and returns how many went.
func deleteSessionsByMethod(ctx context.Context, q conn, userID int64, method string) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND auth_method = ?", userID, method)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deleteSessionsOf ends every session of a person.
func deleteSessionsOf(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// deleteSessionsExceptHash ends every session of a person but the one
// behind a cookie hash.
func deleteSessionsExceptHash(ctx context.Context, q conn, userID int64, keep string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND hash != ?", userID, keep)
	return err
}

// sessionProof reads the sign-in facts of the session a person proves
// themselves with.
func sessionProof(ctx context.Context, q conn, hash, family string, userID int64) (local, method, created, seen, expires string, err error) {
	err = q.QueryRowContext(ctx, `SELECT s.local_auth_at, s.auth_method, s.created_at, s.last_seen_at, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.hash = ? AND s.family = ? AND s.user_id = ? AND u.state = 'active'`,
		hash, family, userID).Scan(&local, &method, &created, &seen, &expires)
	return local, method, created, seen, expires, err
}

// deleteOIDCSessions ends every session that began with an OIDC sign-in and
// returns how many went.
func deleteOIDCSessions(ctx context.Context, q conn) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE auth_method = 'oidc'")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
