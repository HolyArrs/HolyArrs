package requests

// Statements of follows_tmdb.go. The text is the exact SQL the server runs.
const (
	libraryShowSQL         = "SELECT id FROM titles WHERE kind = 'show' AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?"
	followShowTMDBSQL      = "INSERT INTO tmdb_follows (user_id, kind, tmdb_id, created_at) VALUES (?, 'show', ?, ?) ON CONFLICT DO NOTHING"
	unfollowShowTMDBSQL    = "DELETE FROM tmdb_follows WHERE user_id = ? AND kind = 'show' AND tmdb_id = ?"
	unfollowShowTMDBSQL2   = "DELETE FROM follows WHERE user_id = ? AND title_id = ?"
	isFollowingShowTMDBSQL = `SELECT (SELECT count(*) FROM tmdb_follows WHERE user_id = ? AND kind = 'show' AND tmdb_id = ?)
		+ (SELECT count(*) FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = ? AND t.kind = 'show' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') = ?)`
	libraryMovieSQL         = "SELECT id FROM titles WHERE kind = 'movie' AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?"
	followMovieTMDBSQL      = "INSERT INTO tmdb_movie_follows (user_id, tmdb_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING"
	unfollowMovieTMDBSQL    = "DELETE FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = ?"
	unfollowMovieTMDBSQL2   = "DELETE FROM follows WHERE user_id = ? AND title_id = ?"
	isFollowingMovieTMDBSQL = `SELECT (SELECT count(*) FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = ?)
		+ (SELECT count(*) FROM follows f JOIN titles t ON t.id = f.title_id WHERE f.user_id = ? AND t.kind = 'movie' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') = ?)`
	landMovieFollowsSQL = `SELECT u.id, u.max_rating FROM tmdb_movie_follows f JOIN users u ON u.id = f.user_id
		WHERE f.tmdb_id = ? AND u.state != 'disabled' ORDER BY u.id`
	landMovieFollowsSQL2 = "DELETE FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = ?"
	landMovieFollowsSQL3 = "INSERT INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING"
	tellMovieFollowsSQL  = `SELECT t.id, t.name, f.tmdb_id, u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, '')
		FROM tmdb_movie_follows f JOIN users u ON u.id = f.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		JOIN titles t ON t.kind = 'movie' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') = f.tmdb_id
		JOIN follows fo ON fo.user_id = f.user_id AND fo.title_id = t.id
		WHERE u.state != 'disabled' AND EXISTS (SELECT 1 FROM slots sl JOIN items i ON i.id = sl.item_id
			WHERE i.title_id = t.id AND sl.state IN ('in_library', 'better_wanted'))
		ORDER BY t.id, u.id`
	tellMovieFollowsSQL2 = "DELETE FROM tmdb_movie_follows WHERE user_id = ? AND tmdb_id = ?"
	tellMovieFollowsSQL3 = "INSERT INTO messages (created_at, user_id, kind, text, title_id) VALUES (?, ?, 'ready', ?, ?)"
	landTMDBFollowsSQL   = "SELECT name, coalesce(json_extract(source_ids, '$.tmdb'), ''), kind FROM titles WHERE id = ? AND kind IN ('show', 'movie')"
	landTMDBFollowsSQL2  = `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), coalesce(x.new_episodes, 1), u.max_rating
		FROM tmdb_follows f JOIN users u ON u.id = f.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE f.kind = 'show' AND f.tmdb_id = ? AND u.state != 'disabled' ORDER BY u.id`
	landTMDBFollowsSQL3 = `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, '') FROM users u
			LEFT JOIN user_ask_settings x ON x.user_id = u.id WHERE u.state != 'disabled' AND coalesce(x.new_episodes, 1) = 1 AND u.id IN (SELECT user_id FROM tmdb_follows WHERE kind = 'show' AND tmdb_id = ?) ORDER BY u.id`
	landTMDBFollowsSQL4 = "DELETE FROM tmdb_follows WHERE kind = 'show' AND tmdb_id = ?"
	landTMDBFollowsSQL5 = "INSERT INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING"
	landTMDBFollowsSQL6 = "DELETE FROM tmdb_follows WHERE kind = 'show' AND tmdb_id = ?"
	landTMDBFollowsSQL7 = "INSERT INTO messages (created_at, user_id, kind, text, title_id) VALUES (?, ?, 'new_episodes', ?, ?)"
)
