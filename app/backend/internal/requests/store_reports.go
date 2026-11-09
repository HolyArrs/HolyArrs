package requests

// Statements of reports.go. The text is the exact SQL the server runs.
const (
	reportSQL = `SELECT count(*) FROM slots s JOIN items i ON i.id = s.item_id
		WHERE i.title_id = ? AND s.file_id IS NOT NULL`
	reportSQL2 = "SELECT id FROM items WHERE title_id = ? AND kind = 'movie' ORDER BY id LIMIT 1"
	reportSQL3 = "SELECT count(*) FROM items WHERE title_id = ? AND kind = 'episode' AND json_extract(numbers, '$.season') = ?"
	reportSQL4 = "SELECT count(*) FROM parts WHERE id = ? AND title_id = ? AND kind = 'album'"
	reportSQL5 = "SELECT count(*) FROM versions WHERE id = ? AND title_id = ?"
	reportSQL6 = `SELECT id, version_id, file_id FROM slots WHERE item_id = ? AND file_id IS NOT NULL
			AND (? = 0 OR version_id = ?) ORDER BY id LIMIT 2`
	reportSQL7      = "SELECT count(*) FROM reports WHERE made_by = ? AND state = 'open'"
	reportSlotSQL   = "SELECT id FROM slots WHERE item_id = ? AND version_id = ?"
	tellReporterSQL = `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, '')
		FROM users u LEFT JOIN user_ask_settings x ON x.user_id = u.id WHERE u.id = ?`
	tellReporterSQL2 = "INSERT INTO messages (created_at, user_id, kind, text, title_id, report_id) VALUES (?, ?, 'fixed', ?, ?, ?)"
	openReportsSQL   = "SELECT count(*) FROM reports WHERE state = 'open'"
	reportViewsSQL   = "SELECT name FROM users WHERE id = ?"
	reportViewsSQL2  = "SELECT count(*) FROM versions WHERE title_id = ?"
	reportViewsSQL3  = "SELECT name FROM parts WHERE id = ?"
	reportViewsSQL4  = "SELECT label FROM versions WHERE id = ?"
)
