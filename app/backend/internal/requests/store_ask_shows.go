package requests

// Statements of ask_shows.go. The text is the exact SQL the server runs.
const (
	askShowSQL        = "SELECT keep_up_to_date, new_parts_policy, season_choice FROM titles WHERE id = ?"
	askShowSQL2       = "INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)"
	joinWiderWithSQL  = "SELECT count(*) FROM ask_people WHERE ask_id = ? AND user_id = ?"
	joinWiderWithSQL2 = "INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)"
	regularSeasonsSQL = "SELECT number FROM parts WHERE title_id = ? AND kind = 'season' AND number > 0 AND dropped_at IS NULL ORDER BY number"
	notKeptSQL        = "SELECT keep_up_to_date FROM titles WHERE id = ?"
	notKeptSQL2       = "SELECT keep_up_to_date FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?"
	showReadySQL      = `SELECT count(*), coalesce(sum(CASE WHEN s.state IN ('in_library', 'better_wanted') THEN 1 ELSE 0 END), 0)
		FROM items i JOIN parts p ON p.id = i.part_id
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = ?
		WHERE i.title_id = ? AND i.kind = 'episode' AND i.dropped_at IS NULL AND (`
	addShowForAskSQL  = "SELECT keep_up_to_date FROM titles WHERE id = ?"
	addShowForAskSQL2 = "SELECT keep_up_to_date FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?"
)
