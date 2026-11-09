package requests

// Statements of ask_episodes.go. The text is the exact SQL the server runs.
const (
	episodesNotKeptSQL  = "SELECT keep_up_to_date FROM titles WHERE id = ?"
	episodesNotKeptSQL2 = `SELECT p.keep_up_to_date AND i.keep_up_to_date FROM items i JOIN parts p ON p.id = i.part_id
				WHERE i.title_id = ? AND i.kind = 'episode' AND i.dropped_at IS NULL AND p.kind = 'season' AND p.number = ? AND json_extract(i.numbers, '$.episode') = ?`
)
