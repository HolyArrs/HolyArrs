package requests

// Statements of views.go. The text is the exact SQL the server runs.
const (
	viewsSQL            = "SELECT name FROM users WHERE id = ?"
	viewsSQL2           = "SELECT display_name FROM user_profiles WHERE user_id = ?"
	viewsSQL3           = "SELECT state FROM titles WHERE id = ?"
	addAlsoDeclinedSQL  = "SELECT name FROM users WHERE id = ?"
	addAlsoDeclinedSQL2 = "SELECT display_name FROM user_profiles WHERE user_id = ?"
	alsoDeclinedSQL     = "SELECT decided_by FROM asks WHERE kind = ? AND "
	messagesSQL         = "SELECT count(*) FROM messages WHERE user_id = ? AND read_at IS NULL"
	messagesSQL2        = "SELECT id, created_at, kind, text, ask_id, title_id, read_at, report_id FROM messages WHERE user_id = ? ORDER BY id DESC LIMIT ?"
	markReadSQL         = "UPDATE messages SET read_at = ? WHERE user_id = ? AND read_at IS NULL"
)
