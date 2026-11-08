package requests

// Statements of outside_lists_sync.go. The text is the exact SQL the server runs.
const (
	addShowSQL = "SELECT name FROM titles WHERE id = ?"
	recordSQL  = "UPDATE outside_lists SET last_synced_at = ?, last_result = ?, last_error = ?, updated_at = ? WHERE id = ?"
)
