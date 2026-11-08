package requests

// Statements of ask_music.go. The text is the exact SQL the server runs.
const (
	askArtistSQL     = "INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)"
	albumsNotKeptSQL = "SELECT keep_up_to_date FROM titles WHERE id = ?"
	musicReadySQL    = `SELECT count(*), coalesce(sum(CASE WHEN s.state IN ('in_library', 'better_wanted') THEN 1 ELSE 0 END), 0)
			FROM items i LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = i.title_id ORDER BY is_main DESC, id LIMIT 1)
			WHERE i.part_id = ? AND i.kind = 'track' AND i.dropped_at IS NULL AND i.keep_up_to_date = 1`
	addArtistForAskSQL = "SELECT keep_up_to_date FROM titles WHERE id = ?"
)
