package requests

// Statements of steps.go. The text is the exact SQL the server runs.
const (
	stepOfSQL = `SELECT s.id, s.state, COALESCE(s.last_reason, ''), COALESCE(i.part_id, 0), i.last_searched_at
		FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id JOIN titles t ON t.id = i.title_id
		LEFT JOIN parts p ON p.id = i.part_id
		WHERE i.title_id = ? AND i.keep_up_to_date = 1 AND i.dropped_at IS NULL AND v.keep_up_to_date = 1 AND t.state = 'active'
		AND (t.kind = 'movie' OR t.keep_up_to_date = 1)
		AND (i.part_id IS NULL OR (p.keep_up_to_date = 1 AND p.dropped_at IS NULL AND (t.kind != 'artist' OR p.included = 1)))`
	stepOfSQL2 = `(SELECT v4.id FROM versions v4 LEFT JOIN pick_rules pr ON pr.id = v4.pick_rules_id
		WHERE v4.title_id = i.title_id AND (v4.label = '4K' COLLATE NOCASE OR pr.name = '4K'))`
	stepOfSQL3      = "SELECT next_run, enabled FROM schedules WHERE kind = 'search_missing'"
	searchQueuedSQL = `SELECT count(*) FROM jobs WHERE state IN ('queued', 'running') AND (
		dedupe_key IN (?, ?)
		OR (kind IN ('refresh_title', 'scan_title') AND json_extract(subject, '$.type') = 'title' AND json_extract(subject, '$.id') = ? AND params LIKE '%search%')`
)
