package requests

// Statements of narrow.go. The text is the exact SQL the server runs.
const (
	applySongsSQL  = "SELECT id FROM parts WHERE title_id = ? AND kind = 'album' AND json_extract(source_ids, '$.musicbrainz') = ?"
	applySongsSQL2 = "SELECT count(*) FROM items WHERE part_id = ? AND kind = 'track' AND recording_id IS NOT NULL"
	applySongsSQL3 = "UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'track' AND (recording_id IS NULL OR lower(recording_id) NOT IN ("
)
