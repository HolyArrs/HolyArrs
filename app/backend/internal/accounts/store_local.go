package accounts

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// activeCaller reads an active person into who and returns their stored
// permissions; sql.ErrNoRows when there is no such person or they are turned off.
func activeCaller(ctx context.Context, q conn, id int64, who *webserver.Caller) (perms string, err error) {
	err = q.QueryRowContext(ctx, "SELECT "+userCols+" FROM users u WHERE u.id = ? AND u.state = 'active'", id).
		Scan(&who.UserID, &who.Name, &who.Role, &perms, &who.TwoFactor, &who.MaxRating, &who.RatingCountry)
	return perms, err
}

// localSkipFacts reads what the Kid guard of the local-network skip checks
// on each visit: how many people have an age limit, and how many unused
// invites with one have not run out by now.
func localSkipFacts(ctx context.Context, q conn, now string) (kids, kidInvites int, err error) {
	err = q.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM users WHERE max_rating != ''),
		(SELECT count(*) FROM invites WHERE used_at IS NULL AND max_rating != '' AND expires_at > ?)`, now).
		Scan(&kids, &kidInvites)
	return kids, kidInvites, err
}
