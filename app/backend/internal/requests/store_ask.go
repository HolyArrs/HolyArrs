package requests

// Statements of ask.go. The text is the exact SQL the server runs.
const (
	libraryOverLimitSQL = "SELECT rating FROM titles WHERE id = ?"
	personLimitsSQL     = "SELECT per_week, movies_per_week, shows_per_week, music_per_week FROM user_ask_settings WHERE user_id = ?"
	kindLimitSQL        = `SELECT p.asked_at FROM ask_people p JOIN asks a ON a.id = p.ask_id
		WHERE p.user_id = ? AND p.counted = 1 AND p.asked_at > ? AND a.state != 'turned_down' AND a.kind = ? ORDER BY p.asked_at`
	shownAsSQL     = "SELECT display_name FROM user_profiles WHERE user_id = ?"
	copyOfSQL      = "SELECT id FROM versions WHERE title_id = ? AND four_k = ? ORDER BY is_main DESC, id LIMIT 1"
	fourKRulesSQL  = "SELECT id FROM pick_rules WHERE name = ? ORDER BY id LIMIT 1"
	plainRulesSQL  = "SELECT coalesce(default_pick_rules_id, 0) FROM library_folders WHERE id = ?"
	plainRulesSQL2 = "SELECT id FROM pick_rules WHERE name = ? ORDER BY id LIMIT 1"
	mainFolderSQL  = "SELECT library_folder_id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1"
	askSQL         = "SELECT id FROM asks WHERE kind = 'movie' AND tmdb_id = ? AND quality = ? AND state IN ('asked', 'approved')"
	askSQL2        = "SELECT count(*) FROM slots WHERE state = 'in_library' AND item_id IN (SELECT id FROM items WHERE title_id = ?) AND version_id = ?"
	askSQL3        = "INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)"
	joinSQL        = "SELECT count(*) FROM ask_people WHERE ask_id = ? AND user_id = ?"
	joinSQL2       = "INSERT OR IGNORE INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)"
	decideSQL      = "SELECT count(*) FROM asks WHERE kind = ? AND tmdb_id = ? AND quality = ? AND state IN ('asked', 'approved') AND id != ?"
	decideSQL2     = "SELECT count(*) FROM asks WHERE kind = 'artist' AND json_extract(source_ids, '$.musicbrainz') = ? AND state IN ('asked', 'approved') AND id != ?"
	decideSQL3     = "UPDATE asks SET state = 'approved' WHERE id = ? AND state IN ('asked', 'turned_down')"
	decideSQL4     = "UPDATE asks SET title_id = ? WHERE id = ?"
	setAudioSQL    = "SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1"
	askedRulesSQL  = "SELECT count(*) FROM pick_rules WHERE id = ?"
	addCopySQL     = "SELECT pick_rules_id FROM versions WHERE id = ?"
	turnDownSQL    = "UPDATE asks SET state = 'turned_down' WHERE id = ? AND (state = 'asked' OR (state = 'approved' AND title_id IS NULL))"
	turnDownSQL2   = "SELECT count(*) FROM asks WHERE id = ?"
	withdrawSQL    = "SELECT state, asked_by FROM asks WHERE id = ?"
	withdrawSQL2   = "SELECT user_id FROM ask_people WHERE ask_id = ? ORDER BY asked_at, user_id"
	withdrawSQL3   = "DELETE FROM ask_people WHERE ask_id = ? AND user_id = ?"
	withdrawSQL4   = "UPDATE asks SET asked_by = ? WHERE id = ?"
	withdrawSQL5   = "UPDATE ask_people SET counted = 1 WHERE ask_id = ? AND user_id = ?"
	withdrawSQL6   = "DELETE FROM asks WHERE id = ? AND state = 'asked'"
	checkReadySQL  = `kind = 'movie' AND state = 'approved' AND title_id IS NOT NULL AND EXISTS (
		SELECT 1 FROM slots sl JOIN items i ON i.id = sl.item_id WHERE i.title_id = asks.title_id AND sl.state IN ('in_library', 'better_wanted')
		AND sl.version_id IN (SELECT v.id FROM versions v WHERE v.title_id = asks.title_id AND v.four_k = asks.four_k)) ORDER BY id`
	checkReadySQL2 = "UPDATE asks SET state = 'available', available_at = ?, updated_at = ? WHERE id = ? AND state = 'approved'"
	peopleSQL      = `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), p.asked_at,
			EXISTS (SELECT 1 FROM plex_watchlist_items w WHERE w.user_id = p.user_id AND w.ask_id = p.ask_id AND w.state IN ('asked', 'joined'))
		FROM ask_people p JOIN users u ON u.id = p.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE p.ask_id = ? ORDER BY p.asked_at, u.id`
	tellSQL = "INSERT INTO messages (created_at, user_id, kind, text, ask_id, title_id) VALUES (?, ?, ?, ?, ?, ?)"
)
