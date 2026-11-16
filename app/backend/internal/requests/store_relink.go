package requests

// Statements of relink.go. The text is the exact SQL the server runs.
const (
	// relinkSQL pairs each approved or ready movie or show ask that lost its
	// title (never linked, or its title was deleted from the Library) with
	// a Library title of the same kind and the same TMDB id (or, for a show,
	// the same TVDB id).
	relinkSQL = `SELECT a.id, t.id FROM asks a JOIN titles t ON t.kind = a.kind AND t.state != 'removed'
		AND ((a.tmdb_id != '' AND json_extract(t.source_ids, '$.tmdb') = a.tmdb_id)
			OR (a.kind = 'show' AND json_extract(a.source_ids, '$.tvdb') IS NOT NULL AND json_extract(t.source_ids, '$.tvdb') = json_extract(a.source_ids, '$.tvdb')))
		WHERE a.kind IN ('movie', 'show') AND a.state IN ('approved', 'available')
		AND (a.title_id IS NULL OR a.title_id IN (SELECT id FROM titles WHERE state = 'removed'))`
	relinkTitleSQL = " AND t.id = ?"
	relinkOrderSQL = " ORDER BY a.id, t.id"
	relinkSetSQL   = `UPDATE asks SET title_id = ?, updated_at = ? WHERE id = ?
		AND (title_id IS NULL OR title_id IN (SELECT id FROM titles WHERE state = 'removed'))`
)
