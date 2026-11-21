package accounts

import (
	"context"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// PersonAccess is what a background job checks before it acts for a
// person: who they are and what they may do now.
type PersonAccess struct {
	Name, Role, MaxRating string
	// Perms are the permissions the person holds; the owner holds them all.
	Perms []webserver.Perm
	// Removed is set once the person was removed.
	Removed bool
}

// ReadPersonAccess reads one person's access; sql.ErrNoRows when there is
// no such person.
func ReadPersonAccess(ctx context.Context, q database.Querier, userID int64) (PersonAccess, error) {
	var p PersonAccess
	var perms, removedAt string
	err := q.QueryRowContext(ctx, "SELECT name, role, max_rating, perms, removed_at FROM users WHERE id = ?", userID).
		Scan(&p.Name, &p.Role, &p.MaxRating, &perms, &removedAt)
	if err != nil {
		return PersonAccess{}, err
	}
	p.Removed = removedAt != ""
	if p.Role == webserver.RoleOwner {
		p.Perms = slices.Clone(webserver.AllPerms)
	} else {
		p.Perms = parsePerms(perms)
	}
	return p, nil
}
