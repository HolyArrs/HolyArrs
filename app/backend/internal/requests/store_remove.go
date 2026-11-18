package requests

// Statements of remove.go. The text is the exact SQL the server runs.
const (
	removeSQL  = "DELETE FROM asks WHERE id = ?"
	removeSQL2 = "a.state = 'approved'"
	removeSQL3 = "SELECT id FROM asks WHERE state IN ('available', 'turned_down') ORDER BY id"
)
