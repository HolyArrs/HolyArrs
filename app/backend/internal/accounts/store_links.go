package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// plexLinkRow is the token reference of a person's Plex link with the
// person's role and state.
func plexLinkRow(ctx context.Context, q conn, userID int64) (ref sql.NullInt64, role, state string, err error) {
	err = q.QueryRowContext(ctx, `SELECT l.token_secret_id, u.role, u.state FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = 'plex' AND l.user_id = ?`, userID).Scan(&ref, &role, &state)
	return ref, role, state, err
}

// plexTokenRef is the token reference of one Plex link of a person.
func plexTokenRef(ctx context.Context, q conn, extID string, userID int64) (sql.NullInt64, error) {
	var ref sql.NullInt64
	err := q.QueryRowContext(ctx, "SELECT token_secret_id FROM user_links WHERE provider = 'plex' AND external_id = ? AND user_id = ?",
		extID, userID).Scan(&ref)
	return ref, err
}

// setPlexTokenRef points one Plex link at a stored token and returns how
// many links it changed.
func setPlexTokenRef(ctx context.Context, q conn, secretID int64, extID string, userID int64) (int64, error) {
	res, err := q.ExecContext(ctx, "UPDATE user_links SET token_secret_id = ? WHERE provider = 'plex' AND external_id = ? AND user_id = ?", secretID, extID, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// tokenIDs reads the secret ids a set of links keeps their tokens under.
func tokenIDs(ctx context.Context, q conn, query string, args ...any) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// plexTokenIDsOfLink lists the token ids kept on one Plex link of a person.
func plexTokenIDsOfLink(ctx context.Context, q conn, extID string, userID int64) ([]int64, error) {
	return tokenIDs(ctx, q, "SELECT token_secret_id FROM user_links WHERE token_secret_id IS NOT NULL AND provider = 'plex' AND external_id = ? AND user_id = ?", extID, userID)
}

// plexTokenIDsOfUser lists the token ids kept on a person's Plex links.
func plexTokenIDsOfUser(ctx context.Context, q conn, userID int64) ([]int64, error) {
	return tokenIDs(ctx, q, "SELECT token_secret_id FROM user_links WHERE token_secret_id IS NOT NULL AND provider = 'plex' AND user_id = ?", userID)
}

// tokenIDsOfUser lists the token ids kept on all of a person's links.
func tokenIDsOfUser(ctx context.Context, q conn, userID int64) ([]int64, error) {
	return tokenIDs(ctx, q, "SELECT token_secret_id FROM user_links WHERE token_secret_id IS NOT NULL AND user_id = ?", userID)
}

// plexLinkOwner is the person a Plex account is linked to.
func plexLinkOwner(ctx context.Context, q conn, extID string) (int64, error) {
	var other int64
	err := q.QueryRowContext(ctx, "SELECT user_id FROM user_links WHERE provider = 'plex' AND external_id = ?", extID).Scan(&other)
	return other, err
}

// plexExternalID is the Plex account a person is linked to.
func plexExternalID(ctx context.Context, q conn, userID int64) (string, error) {
	var ext string
	err := q.QueryRowContext(ctx, "SELECT external_id FROM user_links WHERE provider = 'plex' AND user_id = ?", userID).Scan(&ext)
	return ext, err
}

// deletePlexLink removes a person's Plex link and returns how many went.
func deletePlexLink(ctx context.Context, q conn, userID int64) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM user_links WHERE provider = 'plex' AND user_id = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// insertPlexLink links a Plex account to a person; ref is the stored token
// id or nil.
func insertPlexLink(ctx context.Context, q conn, userID int64, extID, name, at string, ref any) error {
	_, err := q.ExecContext(ctx, "INSERT INTO user_links(user_id, provider, external_id, external_name, created_at, token_secret_id) VALUES (?, 'plex', ?, ?, ?, ?)",
		userID, extID, name, at, ref)
	return err
}

// plexLinkName is the Plex name and token reference of a person's link.
func plexLinkName(ctx context.Context, q conn, userID int64) (name string, ref sql.NullInt64, err error) {
	err = q.QueryRowContext(ctx, "SELECT external_name, token_secret_id FROM user_links WHERE provider = 'plex' AND user_id = ?",
		userID).Scan(&name, &ref)
	return name, ref, err
}

// otherWaysIn counts a person's links that are not Plex plus their
// single sign-on links.
func otherWaysIn(ctx context.Context, q conn, userID int64) (int, error) {
	var others int
	err := q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM user_links WHERE user_id = ? AND provider <> 'plex') +
			(SELECT count(*) FROM sso_links WHERE user_id = ?)`, userID, userID).Scan(&others)
	return others, err
}

// deleteLinksOf removes every media server link of a person.
func deleteLinksOf(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM user_links WHERE user_id = ?", userID)
	return err
}

// deleteSSOLinksOf removes every single sign-on link of a person.
func deleteSSOLinksOf(ctx context.Context, q conn, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sso_links WHERE user_id = ?", userID)
	return err
}

// eachUserLink calls fn for every media server link, by provider.
func eachUserLink(ctx context.Context, q conn, fn func(userID int64, l LinkView)) error {
	links, err := q.QueryContext(ctx, "SELECT user_id, provider, external_name FROM user_links ORDER BY provider")
	if err != nil {
		return err
	}
	defer links.Close()
	for links.Next() {
		var id int64
		var l LinkView
		if links.Scan(&id, &l.Provider, &l.Name) == nil {
			fn(id, l)
		}
	}
	return nil
}

// eachSSOLink calls fn for every single sign-on link, by kind and issuer.
func eachSSOLink(ctx context.Context, q conn, fn func(userID int64, l SSOLinkView)) error {
	sso, err := q.QueryContext(ctx, "SELECT user_id, kind, issuer, display_name, created_at FROM sso_links ORDER BY kind, issuer")
	if err != nil {
		return err
	}
	defer sso.Close()
	for sso.Next() {
		var id int64
		var l SSOLinkView
		var name sql.NullString
		if sso.Scan(&id, &l.Kind, &l.Issuer, &name, &l.CreatedAt) == nil {
			l.Name = name.String
			fn(id, l)
		}
	}
	return nil
}

// countLinksOf counts the links to one account of a media server.
func countLinksOf(ctx context.Context, q conn, provider, extID string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM user_links WHERE provider = ? AND external_id = ?", provider, extID).Scan(&n)
	return n, err
}

// insertUserLink links a media server account to a person.
func insertUserLink(ctx context.Context, q conn, userID int64, provider, extID, extName, at string) error {
	_, err := q.ExecContext(ctx, "INSERT INTO user_links(user_id, provider, external_id, external_name, created_at) VALUES (?, ?, ?, ?, ?)",
		userID, provider, extID, extName, at)
	return err
}

// linkedUserRow is the person a media server account is linked to, with
// their state.
func linkedUserRow(ctx context.Context, q conn, provider, extID string) (id int64, name, state string, err error) {
	err = q.QueryRowContext(ctx, `SELECT u.id, u.name, u.state FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_id = ?`, provider, extID).Scan(&id, &name, &state)
	return id, name, state, err
}

// linkedUserName is the name of the person a media server account is linked to.
func linkedUserName(ctx context.Context, q conn, provider, extID string) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT u.name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_id = ?`, provider, extID).Scan(&name)
	return name, err
}

// linkedUserNameByExternalName is the name of the person linked under a
// media server name, the lowest user id first.
func linkedUserNameByExternalName(ctx context.Context, q conn, provider, extName string) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT u.name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_name = ? ORDER BY l.user_id LIMIT 1`, provider, extName).Scan(&name)
	return name, err
}

// plexOwnerName is the Plex name linked to the owner.
func plexOwnerName(ctx context.Context, q conn) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT l.external_name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = 'plex' AND u.role = 'owner' LIMIT 1`).Scan(&name)
	return name, err
}

// linkOwner is the person a media server account is linked to.
func linkOwner(ctx context.Context, q conn, provider, extID string) (int64, error) {
	var other int64
	err := q.QueryRowContext(ctx, "SELECT user_id FROM user_links WHERE provider = ? AND external_id = ?", provider, extID).Scan(&other)
	return other, err
}

// ssoLinkedUser is the person a single sign-on identity is linked to, with
// their state.
func ssoLinkedUser(ctx context.Context, q conn, kind, issuer, subject string) (id int64, name, state string, err error) {
	err = q.QueryRowContext(ctx, `SELECT u.id, u.name, u.state FROM sso_links l JOIN users u ON u.id = l.user_id
			WHERE l.kind = ? AND l.issuer = ? AND l.subject = ?`, kind, issuer, subject).Scan(&id, &name, &state)
	return id, name, state, err
}

// insertSSOLink links a single sign-on identity to a person.
func insertSSOLink(ctx context.Context, q conn, kind, issuer, subject string, userID int64, display, at string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sso_links(kind, issuer, subject, user_id, display_name, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		kind, issuer, subject, userID, display, at)
	return err
}

// countOIDCLinks counts the people linked to an OIDC issuer.
func countOIDCLinks(ctx context.Context, q conn, issuer string) (n int, err error) {
	err = q.QueryRowContext(ctx, "SELECT count(*) FROM sso_links WHERE kind = 'oidc' AND issuer = ?", issuer).Scan(&n)
	return n, err
}

// countForwardLinks counts the people linked to a forward-auth issuer.
func countForwardLinks(ctx context.Context, q conn, issuer string) (n int, err error) {
	err = q.QueryRowContext(ctx, "SELECT count(*) FROM sso_links WHERE kind = 'forward_auth' AND issuer = ?", issuer).Scan(&n)
	return n, err
}

// deleteOIDCLinks removes every link to an OIDC issuer and returns how many went.
func deleteOIDCLinks(ctx context.Context, q conn, issuer string) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM sso_links WHERE kind = 'oidc' AND issuer = ?", issuer)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ssoLinksOfUser lists a person's single sign-on links.
func ssoLinksOfUser(ctx context.Context, q conn, userID int64) ([]SSOLinkView, error) {
	rows, err := q.QueryContext(ctx, "SELECT kind, issuer, display_name, created_at FROM sso_links WHERE user_id = ? ORDER BY kind, issuer", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SSOLinkView{}
	for rows.Next() {
		var l SSOLinkView
		var name sql.NullString
		if rows.Scan(&l.Kind, &l.Issuer, &name, &l.CreatedAt) == nil {
			l.Name = name.String
			out = append(out, l)
		}
	}
	return out, nil
}

// waysInBesidesSSO counts a person's media server links and their single
// sign-on links other than one.
func waysInBesidesSSO(ctx context.Context, q conn, userID int64, kind, issuer string) (int, error) {
	var others int
	err := q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM user_links WHERE user_id = ?) +
			(SELECT count(*) FROM sso_links WHERE user_id = ? AND NOT (kind = ? AND issuer = ?))`, userID, userID, kind, issuer).Scan(&others)
	return others, err
}

// deleteSSOLink removes one single sign-on link of a person and returns how
// many went.
func deleteSSOLink(ctx context.Context, q conn, userID int64, kind, issuer string) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM sso_links WHERE user_id = ? AND kind = ? AND issuer = ?", userID, kind, issuer)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// How LinkImported left an outside account.
const (
	LinkAdded = "added" // linked to the person now
	LinkHad   = "had"   // the person had this link already
	LinkTaken = "taken" // linked to someone else (returned), or the person has another one of this kind
)

// LinkedTo is the person an outside account (plex, jellyfin, emby) is linked
// to, with their name; sql.ErrNoRows when it is linked to no one.
func LinkedTo(ctx context.Context, q database.Querier, provider, extID string) (int64, string, error) {
	var id int64
	var name string
	err := q.QueryRowContext(ctx, `SELECT u.id, u.name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ? AND l.external_id = ?`, provider, extID).Scan(&id, &name)
	return id, name, err
}

// LinkImported links an outside account to a person an import made, the
// way a sign-in with it links it, unless the account is linked already or
// the person has another account of that kind. It never turns anyone on:
// the person's first sign-in with that account does, after its checks
// (waitsForSignIn). It returns how the account was left and the person it
// is linked to.
func LinkImported(ctx context.Context, tx *sql.Tx, userID int64, provider, extID, extName string, at time.Time) (string, int64, error) {
	other, _, err := LinkedTo(ctx, tx, provider, extID)
	switch {
	case err == nil && other == userID:
		return LinkHad, userID, nil
	case err == nil:
		return LinkTaken, other, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", 0, err
	}
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM user_links WHERE user_id = ? AND provider = ?", userID, provider).Scan(&n); err != nil || n > 0 {
		return LinkTaken, userID, err
	}
	if err := insertUserLink(ctx, tx, userID, provider, extID, truncate(extName, 200), database.FormatTime(at)); err != nil {
		return "", 0, err
	}
	return LinkAdded, userID, nil
}

// waitsForSignIn says a turned-off person is one an import made and nobody
// has changed since: no password, never updated after it was made, and
// linked to an outside account. Only such a person is turned on by their
// first sign-in; one the owner turned off, or changed in any way, is not.
func waitsForSignIn(ctx context.Context, q conn, userID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM users u WHERE u.id = ? AND u.state = 'disabled' AND u.password_hash = ''
		AND u.role != 'owner' AND u.updated_at = u.created_at AND EXISTS (SELECT 1 FROM user_links WHERE user_id = u.id)`, userID).Scan(&n)
	return n > 0, err
}

// turnOnImported turns on a person waitsForSignIn found, with Kid limits
// when kidPerms is set; false when they were changed in between.
func turnOnImported(ctx context.Context, q conn, userID int64, kidPerms, now string, by database.Actor) (bool, error) {
	who, _ := json.Marshal(by)
	res, err := q.ExecContext(ctx, `UPDATE users SET state = 'active',
			role = CASE WHEN ? != '' THEN 'member' ELSE role END, perms = CASE WHEN ? != '' THEN ? ELSE perms END,
			max_rating = CASE WHEN ? != '' THEN 'PG' ELSE max_rating END, updated_at = ?, updated_by = ?
		WHERE id = ? AND state = 'disabled' AND password_hash = '' AND role != 'owner' AND updated_at = created_at`,
		kidPerms, kidPerms, kidPerms, kidPerms, now, string(who), userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// LinkedPerson is the person a media server account is linked to, and the
// account's name there (ExtName).
type LinkedPerson struct {
	ID      int64
	Name    string
	ExtName string
}

// LinkedPeople maps every linked account of one media server kind (plex,
// jellyfin, emby) by its id there to its person.
func LinkedPeople(ctx context.Context, q database.Lister, provider string) (map[string]LinkedPerson, error) {
	rows, err := q.QueryContext(ctx, `SELECT l.external_id, l.external_name, u.id, u.name FROM user_links l JOIN users u ON u.id = l.user_id
		WHERE l.provider = ?`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LinkedPerson{}
	for rows.Next() {
		var ext string
		var p LinkedPerson
		if err := rows.Scan(&ext, &p.ExtName, &p.ID, &p.Name); err != nil {
			return nil, err
		}
		out[ext] = p
	}
	return out, rows.Err()
}
