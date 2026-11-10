package requests

// Statements of plex_watchlist.go. The text is the exact SQL the server runs.
const (
	peopleSQL2 = `SELECT x.user_id FROM plex_watchlist_people x JOIN users u ON u.id = x.user_id
		WHERE (x.movies = 1 OR x.shows = 1) AND u.state = 'active' ORDER BY x.user_id`
	peopleSQL3 = `SELECT u.id, coalesce(x.chosen, 0) FROM users u LEFT JOIN plex_watchlist_people x ON x.user_id = u.id
		WHERE u.state = 'active' AND (coalesce(x.chosen, 0) = 0 OR x.movies = 1 OR x.shows = 1) ORDER BY u.id`
	passSQL = `SELECT u.name, u.role, u.perms, u.state, u.max_rating, coalesce(x.movies, 0), coalesce(x.shows, 0), coalesce(x.chosen, 0)
		FROM users u LEFT JOIN plex_watchlist_people x ON x.user_id = u.id WHERE u.id = ?`
	passSQL2 = "SELECT state, tries FROM plex_watchlist_items WHERE user_id = ? AND plex_key = ?"
	passSQL3 = "UPDATE plex_watchlist_items SET seen_at = ? WHERE user_id = ? AND plex_key = ?"
	passSQL4 = `INSERT INTO plex_watchlist_items (user_id, plex_key, kind, ref, name, year, state, ask_id, note, tries, seen_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (user_id, plex_key) DO UPDATE SET ref = excluded.ref, name = excluded.name, year = excluded.year, state = excluded.state,
					ask_id = excluded.ask_id, note = excluded.note, tries = excluded.tries, seen_at = excluded.seen_at, updated_at = excluded.updated_at`
	actSQL = `SELECT a.id FROM asks a JOIN ask_people x ON x.ask_id = a.id
			WHERE a.kind = 'movie' AND a.tmdb_id = ? AND a.four_k = 0 AND x.user_id = ? ORDER BY a.id DESC LIMIT 1`
	actSQL2 = `SELECT a.id FROM asks a JOIN ask_people x ON x.ask_id = a.id
		WHERE a.kind = 'show' AND x.user_id = ? AND (`
	checkedSQL = `INSERT INTO plex_watchlist_people (user_id, checked_at, error) VALUES (?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET checked_at = excluded.checked_at, error = excluded.error`
	readWatchlistSQL  = "SELECT movies, shows, chosen, checked_at, error FROM plex_watchlist_people WHERE user_id = ?"
	readWatchlistSQL2 = `SELECT name, year, kind, state, ask_id, note, seen_at FROM plex_watchlist_items
		WHERE user_id = ? ORDER BY seen_at DESC, updated_at DESC LIMIT ?`
	saveWatchlistSwitchesSQL = `INSERT INTO plex_watchlist_people (user_id, movies, shows, chosen) VALUES (?, ?, ?, 1)
			ON CONFLICT (user_id) DO UPDATE SET movies = excluded.movies, shows = excluded.shows, chosen = 1`
	plexLinkNameSQL = "SELECT external_name FROM user_links WHERE provider = 'plex' AND user_id = ? LIMIT 1"
)
