package accounts

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// ownerIdentity is the id and name of the active owner.
func ownerIdentity(ctx context.Context, q conn) (id int64, name string, err error) {
	err = q.QueryRowContext(ctx, "SELECT id, name FROM users WHERE role = 'owner' AND state = 'active'").Scan(&id, &name)
	return id, name, err
}

// countOwners counts the owner rows, in any state.
func countOwners(ctx context.Context, q conn) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role = 'owner'").Scan(&n)
	return n, err
}

// insertUserTx stores a new person and returns their id.
func insertUserTx(tx *sql.Tx, u *domain.User, by domain.Actor) (int64, error) {
	return database.Insert(tx, "users", u, by)
}

// loginRow reads what a password sign-in checks for a typed name.
func loginRow(ctx context.Context, q conn, name string) (id int64, userName, hash, state string, err error) {
	err = q.QueryRowContext(ctx, "SELECT id, name, password_hash, state FROM users WHERE name = ?", name).
		Scan(&id, &userName, &hash, &state)
	return id, userName, hash, state, err
}

// reauthRow reads the name and password hash of an active person.
func reauthRow(ctx context.Context, q conn, id int64) (name, hash string, err error) {
	err = q.QueryRowContext(ctx,
		"SELECT name, password_hash FROM users WHERE id = ? AND state = 'active'", id).Scan(&name, &hash)
	return name, hash, err
}

// hasPassword says whether a person has a password set.
func hasPassword(ctx context.Context, q conn, id int64) (bool, error) {
	var has bool
	err := q.QueryRowContext(ctx, "SELECT password_hash != '' FROM users WHERE id = ?", id).Scan(&has)
	return has, err
}

// nameOfTyped is the stored name a typed sign-in name matches.
func nameOfTyped(ctx context.Context, q conn, typed string) (string, error) {
	var n string
	err := q.QueryRowContext(ctx, "SELECT name FROM users WHERE name = ?", typed).Scan(&n)
	return n, err
}

// nameOfUser is the name of a person.
func nameOfUser(ctx context.Context, q conn, id int64) (string, error) {
	var n string
	err := q.QueryRowContext(ctx, "SELECT name FROM users WHERE id = ?", id).Scan(&n)
	return n, err
}

// roleOfUser is the role of a person.
func roleOfUser(ctx context.Context, q conn, id int64) (string, error) {
	var role string
	err := q.QueryRowContext(ctx, "SELECT role FROM users WHERE id = ?", id).Scan(&role)
	return role, err
}

// realNameRows lists every member name and every "<kind>:<name>" of a
// Jellyfin or Emby account linked to a member.
func realNameRows(ctx context.Context, q conn) (*sql.Rows, error) {
	return q.QueryContext(ctx, `SELECT name FROM users UNION
			SELECT provider || ':' || external_name FROM user_links WHERE provider IN ('jellyfin', 'emby') AND external_name != ''`)
}

// stateOfUser is the state (active or turned off) of a person.
func stateOfUser(ctx context.Context, q conn, id int64) (string, error) {
	var state string
	err := q.QueryRowContext(ctx, "SELECT state FROM users WHERE id = ?", id).Scan(&state)
	return state, err
}

// passwordHashOf is a person's password hash, "" when none is set.
func passwordHashOf(ctx context.Context, q conn, id int64) (string, error) {
	var hash string
	err := q.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash)
	return hash, err
}

// firstOwnerID is the id of the lowest-numbered owner row.
func firstOwnerID(ctx context.Context, q conn) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT id FROM users WHERE role = 'owner' ORDER BY id LIMIT 1").Scan(&id)
	return id, err
}

// resetOwnerRow sets an owner's name and password hash and turns the person on.
func resetOwnerRow(ctx context.Context, q conn, name, hash, now, by string, id int64) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET name = ?, password_hash = ?, state = 'active', updated_at = ?, updated_by = ? WHERE id = ?`,
		name, hash, now, by, id)
	return err
}

// setPasswordHash replaces a person's password hash.
func setPasswordHash(ctx context.Context, q conn, hash, now string, id int64) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", hash, now, id)
	return err
}

// listUserViews lists everyone with how they sign in, owner first.
func listUserViews(ctx context.Context, q conn) ([]UserView, error) {
	rows, err := q.QueryContext(ctx, `SELECT u.id, u.name, u.role, u.state, u.perms, u.totp_secret_id IS NOT NULL,
		u.password_hash != '', u.created_at, (SELECT max(last_seen_at) FROM sessions WHERE user_id = u.id), u.max_rating, u.rating_country,
		(SELECT etag FROM user_avatars WHERE user_id = u.id)
		FROM users u ORDER BY CASE u.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, u.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserView{}
	for rows.Next() {
		var u UserView
		var perms string
		var seen, tag sql.NullString
		if err := rows.Scan(&u.ID, &u.Name, &u.Role, &u.State, &perms, &u.TwoFactor, &u.Password, &u.CreatedAt, &seen, &u.MaxRating, &u.RatingCountry, &tag); err != nil {
			return nil, err
		}
		u.Perms = webserver.Caller{Role: u.Role, Perms: parsePerms(perms)}.Effective()
		u.RatingCountry = countryOr(u.RatingCountry)
		u.Links = []LinkView{}
		u.SSOLinks = []SSOLinkView{}
		u.Avatar = avatarURL(u.ID, tag)
		if seen.Valid {
			u.LastSeen = &seen.String
		}
		out = append(out, u)
	}
	return out, nil
}

// userForEdit reads what an edit of a person starts from.
func userForEdit(ctx context.Context, q conn, id int64) (role, state, perms, maxRating, country string, err error) {
	err = q.QueryRowContext(ctx, "SELECT role, state, perms, max_rating, rating_country FROM users WHERE id = ?", id).
		Scan(&role, &state, &perms, &maxRating, &country)
	return role, state, perms, maxRating, country, err
}

// updateUserRow sets a person's role, permissions, state and Kid limit.
func updateUserRow(ctx context.Context, q conn, role, perms, state, maxRating, country, now, by string, id int64) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET role = ?, perms = ?, state = ?, max_rating = ?, rating_country = ?, updated_at = ?, updated_by = ? WHERE id = ?",
		role, perms, state, maxRating, country, now, by, id)
	return err
}

// roleAndPerms is a person's role and stored permissions.
func roleAndPerms(ctx context.Context, q conn, id int64) (role, perms string, err error) {
	err = q.QueryRowContext(ctx, "SELECT role, perms FROM users WHERE id = ?", id).Scan(&role, &perms)
	return role, perms, err
}

// countUsersNamed counts the people with an exact name.
func countUsersNamed(ctx context.Context, q conn, name string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE name = ?", name).Scan(&n)
	return n, err
}

// countActiveOwners counts the owner rows that are turned on.
func countActiveOwners(ctx context.Context, q conn) (int, error) {
	var owner int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role = 'owner' AND state = 'active'").Scan(&owner)
	return owner, err
}

// nameAndState is the name and state of a person.
func nameAndState(ctx context.Context, q conn, id int64) (name, state string, err error) {
	err = q.QueryRowContext(ctx, "SELECT name, state FROM users WHERE id = ?", id).Scan(&name, &state)
	return name, state, err
}

// activePasswordHash is the password hash of an active person.
func activePasswordHash(ctx context.Context, q conn, id int64) (string, error) {
	var hash string
	err := q.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ? AND state = 'active'", id).Scan(&hash)
	return hash, err
}
