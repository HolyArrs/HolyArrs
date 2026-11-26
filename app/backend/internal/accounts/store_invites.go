package accounts

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// listInviteViews lists the newest 200 invites with who made and used them.
func listInviteViews(ctx context.Context, q conn) ([]InviteView, error) {
	rows, err := q.QueryContext(ctx, `SELECT i.hash, i.role, i.perms, i.note, c.name, i.created_at, i.expires_at, i.used_at, u.name,
		i.max_rating, i.rating_country, i.max_uses, i.uses
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
		if err := rows.Scan(&v.ID, &v.Role, &perms, &v.Note, &v.CreatedBy, &v.CreatedAt, &v.ExpiresAt, &usedAt, &usedBy, &v.MaxRating, &v.RatingCountry, &v.MaxUses, &v.Uses); err != nil {
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
		v.NeverExpires = v.ExpiresAt == noEnd
		v.Joined = []string{}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Who joined through each link, oldest first.
	at := map[string]int{}
	for i := range out {
		at[out[i].ID] = i
	}
	joined, err := q.QueryContext(ctx, `SELECT x.hash, u.name FROM invite_uses x JOIN users u ON u.id = x.user_id ORDER BY x.used_at, x.user_id`)
	if err != nil {
		return nil, err
	}
	defer joined.Close()
	for joined.Next() {
		var hash, name string
		if err := joined.Scan(&hash, &name); err != nil {
			return nil, err
		}
		if i, ok := at[hash]; ok {
			out[i].Joined = append(out[i].Joined, name)
		}
	}
	return out, joined.Err()
}

// insertInvite stores a new invite under the hash of its token.
func insertInvite(ctx context.Context, q conn, hash, now string, createdBy int64, role, perms, note, expires, maxRating, country string, maxUses int) error {
	_, err := q.ExecContext(ctx, `INSERT INTO invites(hash, created_at, created_by, role, perms, note, expires_at, max_rating, rating_country, max_uses)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hash, now, createdBy, role, perms, note, expires, maxRating, country, maxUses)
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

// deleteOpenInvite takes back an open invite: any for the owner, else the
// person's own. A link nobody used is removed. A link people already joined
// through is ended instead, so the list still names them. It returns how
// many invites it took back.
func deleteOpenInvite(ctx context.Context, q conn, hash, now string, all bool, userID int64) (int64, error) {
	res, err := q.ExecContext(ctx, "UPDATE invites SET used_at = ? WHERE hash = ? AND used_at IS NULL AND uses > 0 AND (? OR created_by = ?)",
		now, hash, all, userID)
	if err != nil {
		return 0, err
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return n, err
	}
	res, err = q.ExecContext(ctx, "DELETE FROM invites WHERE hash = ? AND used_at IS NULL AND (? OR created_by = ?)",
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

// markInviteUsed counts a new person against an open invite and notes who
// they are. The use that fills the link also closes it (used_at), so a link
// for one person ends exactly as it always did. It returns how many invites
// it changed: 0 means the link was no longer open.
func markInviteUsed(ctx context.Context, q conn, now string, userID int64, hash string) (int64, error) {
	res, err := q.ExecContext(ctx, `UPDATE invites SET uses = uses + 1, used_by = ?,
		used_at = CASE WHEN max_uses > 0 AND uses + 1 >= max_uses THEN ? ELSE NULL END
		WHERE hash = ? AND used_at IS NULL AND (max_uses = 0 OR uses < max_uses)`, userID, now, hash)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return n, err
	}
	_, err = q.ExecContext(ctx, "INSERT INTO invite_uses(hash, user_id, used_at) VALUES (?, ?, ?)", hash, userID, now)
	return n, err
}
