package requests

// Statements of remove.go. The text is the exact SQL the server runs.
const (
	removeSQL  = "DELETE FROM asks WHERE id = ?"
	removeSQL2 = "SELECT id FROM asks WHERE state = 'approved' AND title_id IS NULL ORDER BY id"
	removeSQL3 = "SELECT id FROM asks WHERE state IN ('available', 'turned_down') ORDER BY id"
)
