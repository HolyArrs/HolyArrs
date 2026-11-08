package requests

// Statements of follows.go. The text is the exact SQL the server runs.
const (
	followSQL  = "SELECT kind FROM titles WHERE id = ? AND state != 'removed'"
	followSQL2 = "INSERT INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING"
	followSQL3 = `INSERT OR IGNORE INTO tmdb_movie_follows (user_id, tmdb_id, created_at)
			SELECT ?, json_extract(t.source_ids, '$.tmdb'), ? FROM titles t WHERE t.id = ? AND json_extract(t.source_ids, '$.tmdb') IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM slots sl JOIN items i ON i.id = sl.item_id WHERE i.title_id = t.id AND sl.state IN ('in_library', 'better_wanted'))
			AND NOT EXISTS (SELECT 1 FROM asks a JOIN ask_people ap ON ap.ask_id = a.id WHERE a.title_id = t.id AND ap.user_id = ? AND a.state IN ('asked', 'approved'))`
	followAskSQL    = "SELECT title_id FROM asks WHERE id = ?"
	followAskSQL2   = "UPDATE ask_people SET follow = ? WHERE ask_id = ? AND user_id = ?"
	followAskersSQL = `INSERT OR IGNORE INTO follows (user_id, title_id, created_at) SELECT user_id, ?, ? FROM ask_people
			WHERE ask_id = ? AND follow = 1`
	unfollowSQL    = "DELETE FROM follows WHERE user_id = ? AND title_id = ?"
	unfollowSQL2   = `DELETE FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = (SELECT json_extract(source_ids, '$.tmdb') FROM titles WHERE id = ? AND kind = 'movie')`
	isFollowingSQL = "SELECT count(*) FROM follows WHERE user_id = ? AND title_id = ?"
	followingSQL   = `SELECT t.id, t.kind, t.name, t.year, f.created_at FROM follows f JOIN titles t ON t.id = f.title_id
		WHERE f.user_id = ? AND t.state != 'removed' ORDER BY t.sort_name, t.id`
	followingSQL2 = `SELECT 'show', tmdb_id, '', '', created_at FROM tmdb_follows tf WHERE user_id = ? AND kind = 'show'
			AND NOT EXISTS (SELECT 1 FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = tf.user_id AND t.kind = 'show' AND json_extract(t.source_ids, '$.tmdb') = tf.tmdb_id)
		UNION ALL SELECT 'movie', tmdb_id, '', '', created_at FROM tmdb_movie_follows mf WHERE user_id = ?
			AND NOT EXISTS (SELECT 1 FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = mf.user_id AND t.kind = 'movie' AND json_extract(t.source_ids, '$.tmdb') = mf.tmdb_id)
		UNION ALL SELECT 'album', mbid, name, artist, created_at FROM album_follows WHERE user_id = ?
		ORDER BY 1, 5`
	sendAlbumNoticesSQL = `SELECT n.title_id, t.name, count(*) FROM new_album_notices n JOIN titles t ON t.id = n.title_id
		WHERE n.sent_at IS NULL GROUP BY n.title_id HAVING min(n.arrived_at) <= ? ORDER BY n.title_id`
	sendAlbumNoticesSQL2 = "UPDATE new_album_notices SET sent_at = ? WHERE title_id = ? AND sent_at IS NULL"
	sendAlbumNoticesSQL3 = "INSERT INTO messages (created_at, user_id, kind, text, title_id) VALUES (?, ?, 'new_albums', ?, ?)"
	followersSQL         = `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, '')
		FROM follows f JOIN users u ON u.id = f.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE f.title_id = ? AND u.state != 'disabled' AND coalesce(x.new_episodes, 1) = 1 ORDER BY u.id`
)
