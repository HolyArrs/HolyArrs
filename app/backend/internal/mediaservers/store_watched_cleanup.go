package mediaservers

import (
	"context"
	"time"
)

// cleanupAccount is one account in watch_state, with its server's name.
type cleanupAccount struct {
	acc    watchAccount
	name   string
	server string
}

// cleanupPeople lists every account the last watch history read saw.
func cleanupPeople(ctx context.Context, q watchReader) ([]cleanupAccount, error) {
	rows, err := q.QueryContext(ctx, `SELECT ws.media_server_id, ws.account_key, max(ws.account_name), ms.name
		FROM watch_state ws JOIN media_servers ms ON ms.id = ws.media_server_id
		GROUP BY ws.media_server_id, ws.account_key ORDER BY ms.name, 3, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cleanupAccount{}
	for rows.Next() {
		var a cleanupAccount
		if err := rows.Scan(&a.acc.server, &a.acc.key, &a.name, &a.server); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// cleanupSeen is what each account watched and when; a row the server gave
// no date counts from when it was read.
func cleanupSeen(ctx context.Context, q watchReader) (map[watchAccount]map[seenKey]time.Time, error) {
	rows, err := q.QueryContext(ctx, "SELECT media_server_id, account_key, title_id, season, episode, COALESCE(last_watched_at, read_at) FROM watch_state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[watchAccount]map[seenKey]time.Time{}
	for rows.Next() {
		var a watchAccount
		var k seenKey
		var at string
		if err := rows.Scan(&a.server, &a.key, &k.title, &k.season, &k.episode, &at); err != nil {
			return nil, err
		}
		when, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			when = time.Now().UTC()
		}
		if out[a] == nil {
			out[a] = map[seenKey]time.Time{}
		}
		out[a][k] = when
	}
	return out, rows.Err()
}
