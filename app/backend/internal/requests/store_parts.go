package requests

// Statements of parts.go. The text is the exact SQL the server runs.
const (
	backfillPartsSQL = "SELECT count(*) FROM ask_parts WHERE key >= ? AND key < ?"
	partStoredSQL    = "SELECT count(*) FROM ask_parts WHERE key = ?"
	putPartSQL       = "INSERT INTO ask_parts (key, body, fetched_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at"
	askPartsSQL      = "SELECT key, body FROM ask_parts WHERE key >= ? AND key < ?"
	askPartsSQL2     = "SELECT body FROM ask_parts WHERE key = ?"
	askPartsSQL3     = "SELECT body FROM ask_parts WHERE key = ?"
)
