package requests

// Statements of relink.go. The text is the exact SQL the server runs.
const (
	// relinkAsksSQL lists the approved or ready movie and show asks with the
	// ids a Library title can match on.
	relinkAsksSQL = `SELECT id, kind, tmdb_id, COALESCE(CAST(json_extract(source_ids, '$.tvdb') AS TEXT), ''), title_id
		FROM asks WHERE kind IN ('movie', 'show') AND state IN ('approved', 'available') ORDER BY id`
	// relinkSetSQL moves an ask only while it still points where it did
	// when it was read.
	relinkSetSQL = "UPDATE asks SET title_id = ?, updated_at = ? WHERE id = ? AND title_id IS ?"
)
