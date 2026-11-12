package accounts

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// listInviteViews lists the newest 200 invites with who made and used them.
func listInviteViews(ctx context.Context, q conn) ([]InviteView, error) {
	rows, err := q.QueryContext(ctx, `SELECT i.hash, i.role, i.perms, i.note, c.name, i.created_at, i.expires_at, i.used_at, u.name,
		i.max_rating, i.rating_country
		FROM invites i JOIN users c ON c.id = i.created_by LEFT JOIN users u ON u.id = i.used_by ORDER BY i.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InviteView{}
	for rows.Next() {
		var v InviteView
		var perms string
		var usedAt, usedBy sql.NullString
		if err := rows.Scan(&v.ID, &v.Role, &perms, &v.Note, &v.CreatedBy, &v.CreatedAt, &v.ExpiresAt, &usedAt, &usedBy, &v.MaxRating, &v.RatingCountry); err != nil {
			return nil, err
		}
		v.Perms = parsePerms(perms)
		v.RatingCountry = countryOr(v.RatingCountry)
		if usedAt.Valid {
			v.UsedAt = &usedAt.String
		}
		if usedBy.Valid {
			v.UsedBy = &usedBy.String
		}
		out = append(out, v)
	}
	return out, nil
}

// insertInvite stores a new invite under the hash of its token.
func insertInvite(ctx context.Context, q conn, hash, now string, createdBy int64, role, perms, note, expires, maxRating, country string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO invites(hash, created_at, created_by, role, perms, note, expires_at, max_rating, rating_country)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hash, now, createdBy, role, perms, note, expires, maxRating, country)
	return err
}

// deleteOpenInvites removes every unused invite. It returns how many went.
func deleteOpenInvites(ctx context.Context, q conn) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM invites WHERE used_at IS NULL")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deleteOpenInvite takes back an unused invite: any for the owner, else the
// person's own. It returns how many went.
func deleteOpenInvite(ctx context.Context, q conn, hash string, all bool, userID int64) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM invites WHERE hash = ? AND used_at IS NULL AND (? OR created_by = ?)",
		hash, all, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// openInviteRow reads an unused invite whose maker is still an active owner
// or admin.
func openInviteRow(ctx context.Context, q database.Querier, hash string) (role, perms, note, expires, maxRating, country string, err error) {
	err = q.QueryRowContext(ctx, `SELECT i.role, i.perms, i.note, i.expires_at, i.max_rating, i.rating_country FROM invites i
		JOIN users c ON c.id = i.created_by
		WHERE i.hash = ? AND i.used_at IS NULL AND c.state = 'active' AND c.role IN ('owner', 'admin')`, hash).
		Scan(&role, &perms, &note, &expires, &maxRating, &country)
	return role, perms, note, expires, maxRating, country, err
}

// inviteMakerRow reads an unused invite with its maker's role and
// permissions, inside a write.
func inviteMakerRow(ctx context.Context, q conn, hash string) (role, perms, expires, maxRating, country, byRole, byPerms string, err error) {
	err = q.QueryRowContext(ctx, `SELECT i.role, i.perms, i.expires_at, i.max_rating, i.rating_country, c.role, c.perms FROM invites i JOIN users c ON c.id = i.created_by
		WHERE i.hash = ? AND i.used_at IS NULL AND c.state = 'active' AND c.role IN ('owner', 'admin')`, hash).
		Scan(&role, &perms, &expires, &maxRating, &country, &byRole, &byPerms)
	return role, perms, expires, maxRating, country, byRole, byPerms, err
}

// markInviteUsed marks an invite used by a new person and returns how many
// invites it changed.
func markInviteUsed(ctx context.Context, q conn, now string, userID int64, hash string) (int64, error) {
	res, err := q.ExecContext(ctx, "UPDATE invites SET used_at = ?, used_by = ? WHERE hash = ? AND used_at IS NULL", now, userID, hash)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
