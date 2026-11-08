package requests

// Statements of outside_lists.go. The text is the exact SQL the server runs.
const (
	removeOutsideListSQL = "DELETE FROM outside_lists WHERE id = ?"
	fillSQL              = "SELECT id FROM library_folders WHERE kind = ? ORDER BY id LIMIT 1"
	fillSQL2             = "SELECT count(*) FROM outside_lists WHERE name = ? COLLATE NOCASE AND id != ?"
)
