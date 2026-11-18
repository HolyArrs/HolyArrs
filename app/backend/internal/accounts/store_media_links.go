package accounts

import "context"

// mediaLinksOf lists a person's Jellyfin and Emby links.
func mediaLinksOf(ctx context.Context, q conn, userID int64) ([]LinkView, error) {
	rows, err := q.QueryContext(ctx, `SELECT provider, external_name FROM user_links
		WHERE user_id = ? AND provider IN ('jellyfin', 'emby') ORDER BY provider`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LinkView{}
	for rows.Next() {
		var l LinkView
		if err := rows.Scan(&l.Provider, &l.Name); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// deleteMediaLink removes a person's link of one kind and returns how many went.
func deleteMediaLink(ctx context.Context, q conn, userID int64, provider string) (int64, error) {
	res, err := q.ExecContext(ctx, "DELETE FROM user_links WHERE user_id = ? AND provider = ?", userID, provider)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// waysInBesidesLink counts a person's links of other kinds plus their single
// sign-on links.
func waysInBesidesLink(ctx context.Context, q conn, userID int64, provider string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM user_links WHERE user_id = ? AND provider <> ?) +
			(SELECT count(*) FROM sso_links WHERE user_id = ?)`, userID, provider, userID).Scan(&n)
	return n, err
}
