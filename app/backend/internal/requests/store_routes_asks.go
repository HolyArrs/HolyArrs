package requests

// Statements of routes_asks.go. The text is the exact SQL the server runs.
const (
	askMineSQL       = "EXISTS (SELECT 1 FROM ask_people p WHERE p.ask_id = a.id AND p.user_id = ?)"
	askLimitUsersSQL = `SELECT id, name, role, perms FROM users ORDER BY name`
	askWaitingSQL    = "SELECT count(*) FROM asks WHERE state = 'asked'"
)
